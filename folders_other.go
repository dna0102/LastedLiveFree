//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// mediaFolder returns ~/Pictures, or ~/Movies (macOS) / ~/Videos for video.
func mediaFolder(kind string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	name := "Pictures"
	if kind == "video" {
		name = "Movies"
		if _, err := os.Stat(filepath.Join(home, name)); err != nil {
			name = "Videos"
		}
	}
	return filepath.Join(home, name)
}
