// Package hardware detects the CPU and GPUs, mainly to pick the right video
// encoder for the machine.
package hardware

import (
	"runtime"
	"sort"
	"strings"
	"sync"
)

type GPU struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"` // nvidia | amd | intel | apple | other
	VRAMMB int    `json:"vram_mb,omitempty"`
}

type Info struct {
	CPU     string `json:"cpu"`
	Threads int    `json:"threads"`
	GPUs    []GPU  `json:"gpus"`
}

var (
	once   sync.Once
	cached Info
)

// Detect returns the machine's CPU and GPUs. The result is cached; hardware
// doesn't change while the app runs.
func Detect() Info {
	once.Do(func() {
		cached = Info{CPU: cpuName(), Threads: runtime.NumCPU(), GPUs: gpus()}
		if cached.CPU == "" {
			cached.CPU = runtime.GOARCH + " CPU"
		}
		// Dedicated cards first, so the UI and encoder choice lead with them.
		sort.SliceStable(cached.GPUs, func(i, j int) bool {
			return vendorRank(cached.GPUs[i].Vendor) < vendorRank(cached.GPUs[j].Vendor)
		})
	})
	return cached
}

// Encoders lists the ffmpeg hardware encoders for the detected GPUs, best first.
func (i Info) Encoders() []string {
	var out []string
	for _, g := range i.GPUs {
		if enc := encoderFor(g.Vendor); enc != "" {
			out = append(out, enc)
		}
	}
	return out
}

func encoderFor(vendor string) string {
	switch vendor {
	case "nvidia":
		return "h264_nvenc"
	case "amd":
		return "h264_amf"
	case "intel":
		return "h264_qsv"
	case "apple":
		return "h264_videotoolbox"
	}
	return ""
}

func vendorRank(v string) int {
	switch v {
	case "nvidia":
		return 0
	case "amd":
		return 1
	case "apple":
		return 2
	case "intel":
		return 3
	}
	return 4
}

func vendorOf(pciID uint32, name string) string {
	switch pciID {
	case 0x10de:
		return "nvidia"
	case 0x1002, 0x1022:
		return "amd"
	case 0x8086:
		return "intel"
	}
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "nvidia") || strings.Contains(low, "geforce"):
		return "nvidia"
	case strings.Contains(low, "amd") || strings.Contains(low, "radeon"):
		return "amd"
	case strings.Contains(low, "intel"):
		return "intel"
	case strings.Contains(low, "apple"):
		return "apple"
	}
	return "other"
}
