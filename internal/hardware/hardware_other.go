//go:build !windows

package hardware

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func cpuName() string {
	switch runtime.GOOS {
	case "darwin":
		out, _ := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		return strings.TrimSpace(string(out))
	case "linux":
		raw, _ := os.ReadFile("/proc/cpuinfo")
		for _, line := range strings.Split(string(raw), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// gpus only knows about Apple Silicon outside Windows; everything else falls
// back to the encoder test in ffmpeg.
func gpus() []GPU {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return []GPU{{Name: "Apple GPU", Vendor: "apple"}}
	}
	return nil
}
