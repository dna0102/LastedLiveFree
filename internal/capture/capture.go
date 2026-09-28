// Package capture lists and captures what a screen source can show: monitors
// (captured by ffmpeg's ddagrab) and individual application windows (captured
// here with PrintWindow and fed to ffmpeg).
package capture

// Monitor is a display; Index is its ddagrab output_idx.
type Monitor struct {
	Index  int    `json:"index"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Label  string `json:"label"`
}

type Window struct {
	HWND   string `json:"hwnd"` // hex handle, e.g. 0x1a2b3c
	Title  string `json:"title"`
	App    string `json:"app"` // executable name, e.g. chrome.exe
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
