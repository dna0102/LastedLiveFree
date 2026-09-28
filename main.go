// Lasted Live Free is the single-account edition of Lasted Live, a desktop
// studio for going LIVE on TikTok.
//
// The UI is a Wails webview (WebView2 on Windows, WKWebView on macOS). The API
// is served by the webview's in-process asset handler, so nothing listens on
// a local port.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"lastedlive/internal/encoder"
	"lastedlive/internal/ffsetup"
	"lastedlive/internal/manager"
	"lastedlive/internal/server"
	"lastedlive/internal/store"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	st, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}
	// Use an ffmpeg that's already on the machine, else the one we installed
	// on an earlier run. If neither exists the UI offers to download it.
	setup := ffsetup.New(st.Dir())
	ff := encoder.FindFFmpeg(os.Getenv("FFMPEG_PATH"))
	if ff == "" {
		ff = setup.Installed()
	}
	m := manager.New(st, ff)
	setup.OnDone = m.SetFFmpeg
	app := NewApp(m)

	ui, _ := fs.Sub(assets, "frontend")
	err = wails.Run(&options.App{
		Title:            "Lasted Live Free",
		Width:            1440,
		Height:           900,
		MinWidth:         1180,
		MinHeight:        720,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 255},
		AssetServer: &assetserver.Options{
			Assets:  ui,
			Handler: server.New(m, setup), // /api/*, handled in-process
		},
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind:          []interface{}{app},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WebviewUserDataPath:  st.Dir() + string(os.PathSeparator) + "webview",
			DisablePinchZoom:     true,
			IsZoomControlEnabled: false,
		},
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About:      &mac.AboutInfo{Title: "Lasted Live Free", Message: "TikTok LIVE studio"},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Lasted Live Free:", err)
		os.Exit(1)
	}
}
