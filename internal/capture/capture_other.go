//go:build !windows

package capture

import "errors"

// Window capture is Windows-only for now.

type Feed struct{}

func (f *Feed) Path() string     { return "" }
func (f *Feed) Size() (int, int) { return 0, 0 }
func (f *Feed) Stop()            {}

func StartFeed(hwnd string, fps int) (*Feed, error) {
	return nil, errors.New("window capture is only available on Windows")
}

func ListWindows() []Window { return nil }

func WindowAlive(hwnd string) bool { return false }

func ResolveWindow(hwnd, app, title string) string { return "" }

func WindowThumb(hwnd string, maxW int) ([]byte, error) {
	return nil, errors.New("window capture is only available on Windows")
}
