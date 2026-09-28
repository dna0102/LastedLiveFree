//go:build windows

package capture

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procStretchBlt         = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
	procGdiFlush           = gdi32.NewProc("GdiFlush")
	errWindowGone          = errors.New("window closed")
	errWindowMinimised     = errors.New("window minimised")
)

const (
	pwClientOnly        = 0x1
	pwRenderFullContent = 0x2 // needed for GPU-composited windows (browsers, Electron); plain GDI capture comes out black
	srccopy             = 0x00CC0020
	halftone            = 4
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// dib is a top-down 32-bit BGRA DIB section with its own memory DC.
type dib struct {
	dc, bmp, old uintptr
	w, h         int
	bits         unsafe.Pointer
}

func newDIB(w, h int) (*dib, error) {
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	bi := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return nil, errors.New("CreateDIBSection failed")
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	return &dib{dc: dc, bmp: bmp, old: old, w: w, h: h, bits: bits}, nil
}

func (d *dib) free() {
	procSelectObject.Call(d.dc, d.old)
	procDeleteObject.Call(d.bmp)
	procDeleteDC.Call(d.dc)
}

func (d *dib) bytes() []byte { return unsafe.Slice((*byte)(d.bits), d.w*d.h*4) }

// WindowGrabber copies a window's client area into a BGRA frame. The frame
// size is fixed when the grabber is created, since ffmpeg's rawvideo input
// can't change size; if the window is resized, it's scaled to fit.
type WindowGrabber struct {
	hwnd windows.HWND
	out  *dib
	tmp  *dib
}

func parseHWND(s string) (windows.HWND, error) {
	var h uintptr
	if _, err := fmt.Sscanf(s, "0x%x", &h); err != nil || h == 0 {
		return 0, fmt.Errorf("bad window handle %q", s)
	}
	return windows.HWND(h), nil
}

func clientSize(h windows.HWND) (int, int) {
	var r rect
	procGetClientRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// NewWindowGrabber uses the window's current client size, rounded down to even
// numbers for yuv420.
func NewWindowGrabber(hwnd string) (*WindowGrabber, error) {
	h, err := parseHWND(hwnd)
	if err != nil {
		return nil, err
	}
	if !windows.IsWindow(h) {
		return nil, errWindowGone
	}
	w, hh := clientSize(h)
	if w < 16 || hh < 16 {
		w, hh = 1280, 720 // minimised; frames arrive once it's restored
	}
	out, err := newDIB(w&^1, hh&^1)
	if err != nil {
		return nil, err
	}
	return &WindowGrabber{hwnd: h, out: out}, nil
}

func (g *WindowGrabber) Size() (int, int) { return g.out.w, g.out.h }

// Grab captures a frame and returns its BGRA pixels, valid until the next call.
// For a minimised window it returns errWindowMinimised and leaves the previous
// frame in place.
func (g *WindowGrabber) Grab() ([]byte, error) {
	if !windows.IsWindow(g.hwnd) {
		return nil, errWindowGone
	}
	if iconic, _, _ := procIsIconic.Call(uintptr(g.hwnd)); iconic != 0 {
		return g.out.bytes(), errWindowMinimised
	}
	w, h := clientSize(g.hwnd)
	if w < 1 || h < 1 {
		return g.out.bytes(), errWindowMinimised
	}
	if w == g.out.w && h == g.out.h {
		procPrintWindow.Call(uintptr(g.hwnd), g.out.dc, pwClientOnly|pwRenderFullContent)
	} else {
		if g.tmp == nil || g.tmp.w != w || g.tmp.h != h {
			if g.tmp != nil {
				g.tmp.free()
			}
			t, err := newDIB(w, h)
			if err != nil {
				return nil, err
			}
			g.tmp = t
		}
		procPrintWindow.Call(uintptr(g.hwnd), g.tmp.dc, pwClientOnly|pwRenderFullContent)
		procSetStretchBltMode.Call(g.out.dc, halftone)
		procStretchBlt.Call(g.out.dc, 0, 0, uintptr(g.out.w), uintptr(g.out.h), g.tmp.dc, 0, 0, uintptr(w), uintptr(h), srccopy)
	}
	procGdiFlush.Call()
	return g.out.bytes(), nil
}

func (g *WindowGrabber) Close() {
	if g.tmp != nil {
		g.tmp.free()
	}
	if g.out != nil {
		g.out.free()
	}
}

// WindowThumb returns a JPEG snapshot of the window, at most maxW wide, for the
// source picker.
func WindowThumb(hwnd string, maxW int) ([]byte, error) {
	g, err := NewWindowGrabber(hwnd)
	if err != nil {
		return nil, err
	}
	defer g.Close()
	px, err := g.Grab()
	if err != nil && !errors.Is(err, errWindowMinimised) {
		return nil, err
	}
	w, h := g.Size()
	return thumbJPEG(px, w, h, maxW)
}
