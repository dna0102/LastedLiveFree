//go:build windows

package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procIsIconic            = user32.NewProc("IsIconic")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procGetAncestor         = user32.NewProc("GetAncestor")
)

const (
	gwlExStyle       = -20
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	dwmwaCloaked     = 14
	gaRoot           = 2
	processQueryInfo = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

type rect struct{ Left, Top, Right, Bottom int32 }

func windowText(h windows.HWND) string {
	n, _, _ := procGetWindowTextLength.Call(uintptr(h))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func exeName(h windows.HWND) string {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(h, &pid); err != nil || pid == 0 {
		return ""
	}
	if pid == uint32(os.Getpid()) {
		return "\x00self"
	}
	ph, err := windows.OpenProcess(processQueryInfo, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(ph)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(ph, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}

// skipApps are shell processes whose windows aren't worth offering.
var skipApps = map[string]bool{"textinputhost.exe": true, "shellexperiencehost.exe": true,
	"searchhost.exe": true, "startmenuexperiencehost.exe": true, "applicationframehost.exe": false}

// ListWindows returns visible, titled top-level windows, roughly what OBS's
// window picker shows. Minimised windows are included; they start showing up
// once restored.
func ListWindows() []Window {
	shell := windows.GetShellWindow()
	var out []Window
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		h := windows.HWND(hwnd)
		if h == shell || !windows.IsWindowVisible(h) {
			return 1
		}
		if root, _, _ := procGetAncestor.Call(hwnd, gaRoot); root != hwnd {
			return 1
		}
		ex, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(^uintptr(-gwlExStyle-1)))
		if ex&wsExToolWindow != 0 || ex&wsExNoActivate != 0 {
			return 1
		}
		var cloaked uint32
		if windows.DwmGetWindowAttribute(h, dwmwaCloaked, unsafe.Pointer(&cloaked), 4) == nil && cloaked != 0 {
			return 1 // suspended UWP apps, windows on other virtual desktops
		}
		title := strings.TrimSpace(windowText(h))
		if title == "" || title == "Program Manager" {
			return 1
		}
		app := exeName(h)
		if app == "\x00self" || skipApps[strings.ToLower(app)] || strings.HasPrefix(strings.ToLower(app), "lastedlive") {
			return 1
		}
		// The client area is what gets captured, so report its size.
		w, hh := clientSize(h)
		if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 || w < 16 || hh < 16 {
			w, hh = 1280, 720 // minimised; matches the grabber's fallback
		} else if w < 80 || hh < 60 {
			return 1
		}
		out = append(out, Window{HWND: fmt.Sprintf("0x%x", hwnd), Title: title, App: app, Width: w &^ 1, Height: hh &^ 1})
		return 1
	})
	_ = windows.EnumWindows(cb, nil)
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].App) < strings.ToLower(out[j].App) })
	return out
}

func WindowAlive(hwnd string) bool {
	var h uintptr
	if _, err := fmt.Sscanf(hwnd, "0x%x", &h); err != nil || h == 0 {
		return false
	}
	return windows.IsWindow(windows.HWND(h))
}
