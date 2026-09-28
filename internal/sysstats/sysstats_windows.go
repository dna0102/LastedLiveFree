//go:build windows

// Package sysstats reports system CPU and memory load for the status bar.
package sysstats

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procSystemTimes  = kernel32.NewProc("GetSystemTimes")
	procMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")

	mu                  sync.Mutex
	lastIdle, lastTotal uint64
)

type fileTime struct{ Low, High uint32 }

func (f fileTime) u64() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// CPUPercent returns CPU usage since the previous call (0 the first time).
func CPUPercent() float64 {
	var idle, kernel, user fileTime
	r, _, _ := procSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0
	}
	i := idle.u64()
	t := kernel.u64() + user.u64() // kernel time includes idle
	mu.Lock()
	defer mu.Unlock()
	di, dt := i-lastIdle, t-lastTotal
	first := lastTotal == 0
	lastIdle, lastTotal = i, t
	if first || dt == 0 {
		return 0
	}
	return float64(dt-di) * 100 / float64(dt)
}

func MemPercent() float64 {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	r, _, _ := procMemoryStatus.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0
	}
	return float64(ms.MemoryLoad)
}
