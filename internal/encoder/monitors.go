package encoder

import (
	"fmt"
	"regexp"
	"runtime"
	"sync"
	"time"

	"lastedlive/internal/capture"
)

var dxgiOutRe = regexp.MustCompile(`Opened dxgi output (\d+) with dimensions (\d+)x(\d+)`)

var (
	monMu    sync.Mutex
	monCache []capture.Monitor
	monAt    time.Time
)

// ListMonitors returns the displays ddagrab can capture. It opens each output
// index in turn until one fails, so the list matches the indexes ddagrab
// actually uses. Results are cached for a minute.
func ListMonitors(ffmpeg string) []capture.Monitor {
	if runtime.GOOS != "windows" || ffmpeg == "" {
		return []capture.Monitor{}
	}
	monMu.Lock()
	defer monMu.Unlock()
	if monCache != nil && time.Since(monAt) < time.Minute {
		return monCache
	}
	out := []capture.Monitor{}
	for i := 0; i < 8; i++ {
		b, _ := run(ffmpeg, "-hide_banner", "-loglevel", "verbose", "-f", "lavfi",
			"-i", fmt.Sprintf("ddagrab=output_idx=%d", i), "-frames:v", "1", "-f", "null", "-")
		m := dxgiOutRe.FindStringSubmatch(string(b))
		if m == nil {
			break
		}
		w, h := atoi(m[2]), atoi(m[3])
		out = append(out, capture.Monitor{Index: i, Width: w, Height: h, Label: fmt.Sprintf("Screen %d", i+1)})
	}
	monCache, monAt = out, time.Now()
	return out
}

func MonitorThumb(ffmpeg string, idx, maxW int) ([]byte, error) {
	return runStdout(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", fmt.Sprintf("ddagrab=output_idx=%d,hwdownload,format=bgra,scale=%d:-2", idx, maxW),
		"-frames:v", "1", "-c:v", "mjpeg", "-q:v", "5", "-f", "image2pipe", "pipe:1")
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
