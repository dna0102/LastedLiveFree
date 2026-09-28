//go:build windows

package capture

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

var feedSeq atomic.Uint64

// Feed sends a window's frames to ffmpeg as raw BGRA over a named pipe. One
// goroutine grabs frames as fast as the rate allows; another writes the latest
// one every 1/fps. That way ffmpeg gets exactly fps frames per second and its
// timestamps stay correct even when a grab is slow or the window is minimised.
type Feed struct {
	path   string
	w, h   int
	pipe   windows.Handle
	g      *WindowGrabber
	mu     sync.Mutex
	frame  []byte
	stop   chan struct{}
	done   sync.WaitGroup
	closed atomic.Bool
}

// Path is the pipe for ffmpeg to read (-f rawvideo -pix_fmt bgra -i <path>).
func (f *Feed) Path() string { return f.path }

func (f *Feed) Size() (int, int) { return f.w, f.h }

// StartFeed starts capturing the window. Frames are written once ffmpeg opens
// the pipe.
func StartFeed(hwnd string, fps int) (*Feed, error) {
	g, err := NewWindowGrabber(hwnd)
	if err != nil {
		return nil, err
	}
	w, h := g.Size()
	path := fmt.Sprintf(`\\.\pipe\lastedlive-%d-%d`, time.Now().UnixNano(), feedSeq.Add(1))
	p, _ := windows.UTF16PtrFromString(path)
	pipe, err := windows.CreateNamedPipe(p, windows.PIPE_ACCESS_OUTBOUND, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT,
		1, uint32(w*h*4), 0, 0, nil)
	if err != nil {
		g.Close()
		return nil, fmt.Errorf("window capture pipe: %w", err)
	}
	f := &Feed{path: path, w: w, h: h, pipe: pipe, g: g, frame: make([]byte, w*h*4), stop: make(chan struct{})}
	if px, err := g.Grab(); err == nil || errors.Is(err, errWindowMinimised) {
		copy(f.frame, px)
	}
	f.done.Add(2)
	go f.grabLoop(fps)
	go f.writeLoop(fps)
	return f, nil
}

func (f *Feed) grabLoop(fps int) {
	defer f.done.Done()
	t := time.NewTicker(time.Second / time.Duration(fps))
	defer t.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-t.C:
		}
		px, err := f.g.Grab()
		if err != nil {
			if errors.Is(err, errWindowGone) {
				return // the writer keeps repeating the last frame
			}
			continue
		}
		f.mu.Lock()
		copy(f.frame, px)
		f.mu.Unlock()
	}
}

func (f *Feed) writeLoop(fps int) {
	defer f.done.Done()
	if err := windows.ConnectNamedPipe(f.pipe, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		return // stopped before ffmpeg connected
	}
	interval := time.Second / time.Duration(fps)
	next := time.Now()
	for {
		select {
		case <-f.stop:
			return
		default:
		}
		f.mu.Lock()
		var n uint32
		err := windows.WriteFile(f.pipe, f.frame, &n, nil)
		f.mu.Unlock()
		if err != nil {
			return // ffmpeg went away
		}
		next = next.Add(interval)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		} else if d < -time.Second {
			next = time.Now() // way behind (e.g. after sleep); don't try to catch up
		}
	}
}

func (f *Feed) Stop() {
	if f.closed.Swap(true) {
		return
	}
	close(f.stop)
	_ = windows.CancelIoEx(f.pipe, nil) // unblock ConnectNamedPipe / WriteFile
	_ = windows.DisconnectNamedPipe(f.pipe)
	_ = windows.CloseHandle(f.pipe)
	waited := make(chan struct{})
	go func() { f.done.Wait(); close(waited) }()
	select {
	case <-waited:
		f.g.Close()
	case <-time.After(2 * time.Second):
		// A loop is still stuck in a syscall. Leaking the DIB is better than
		// freeing it while it may still be in use.
	}
}

// ResolveWindow finds the window for a saved source. Handles don't survive a
// restart of the app being captured, so it falls back to a window of the same
// app with the same title, then to any window of that app. Returns "" if none.
func ResolveWindow(hwnd, app, title string) string {
	if WindowAlive(hwnd) {
		return hwnd
	}
	if app == "" {
		return ""
	}
	var sameApp string
	for _, w := range ListWindows() {
		if !strings.EqualFold(w.App, app) {
			continue
		}
		if w.Title == title {
			return w.HWND
		}
		if sameApp == "" {
			sameApp = w.HWND
		}
	}
	return sameApp
}
