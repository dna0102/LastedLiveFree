//go:build windows

package main

import "golang.org/x/sys/windows"

// mediaFolder returns the user's Pictures or Videos folder. KnownFolderPath
// follows OneDrive redirection, unlike building the path by hand.
func mediaFolder(kind string) string {
	id := windows.FOLDERID_Pictures
	if kind == "video" {
		id = windows.FOLDERID_Videos
	}
	p, err := windows.KnownFolderPath(id, 0)
	if err != nil {
		return ""
	}
	return p
}
