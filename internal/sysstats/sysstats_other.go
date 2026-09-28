//go:build !windows

// Package sysstats reports system CPU and memory load for the status bar.
package sysstats

func CPUPercent() float64 { return 0 }

func MemPercent() float64 { return 0 }
