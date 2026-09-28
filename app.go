package main

import (
	"context"
	"fmt"
	"runtime"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"lastedlive/internal/manager"
	"lastedlive/internal/server"
)

type App struct {
	ctx context.Context
	m   *manager.Manager
}

func NewApp(m *manager.Manager) *App { return &App{m: m} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// The UI opens the file dialog through /api/pick-file rather than the JS
	// bindings, which aren't always injected in production builds.
	server.FilePicker = a.PickFile
}

// beforeClose asks for confirmation if any account is still LIVE.
func (a *App) beforeClose(ctx context.Context) bool {
	n := len(a.m.Sessions())
	if n == 0 {
		return false
	}
	msg := "1 account is LIVE. Quitting will end the LIVE."
	if n > 1 {
		msg = fmt.Sprintf("%d accounts are LIVE. Quitting will end every LIVE.", n)
	}
	res, err := wr.MessageDialog(ctx, wr.MessageDialogOptions{
		Type:          wr.QuestionDialog,
		Title:         "End LIVE and quit?",
		Message:       msg,
		Buttons:       []string{"End LIVE & quit", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false
	}
	return res != "End LIVE & quit" && res != "Yes"
}

// shutdown ends any running LIVEs and stops every ffmpeg process.
func (a *App) shutdown(ctx context.Context) {
	for _, s := range a.m.Sessions() {
		if id, ok := s["account_id"].(string); ok {
			_, _ = a.m.EndLive(id)
		}
	}
	a.m.Shutdown()
}

// PickFile shows the native open dialog. kind is "video" or "image".
func (a *App) PickFile(kind string) (string, error) {
	var filters []wr.FileFilter
	switch kind {
	case "video":
		filters = []wr.FileFilter{{DisplayName: "Videos", Pattern: "*.mp4;*.mov;*.mkv;*.webm;*.avi;*.flv;*.m4v"}}
	case "image":
		filters = []wr.FileFilter{{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"}}
	}
	return wr.OpenFileDialog(a.ctx, wr.OpenDialogOptions{Title: "Choose a " + kind, Filters: filters,
		DefaultDirectory: mediaFolder(kind)})
}

// Platform returns runtime.GOOS; the UI hides its window buttons on macOS.
func (a *App) Platform() string { return runtime.GOOS }
