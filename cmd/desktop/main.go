//go:build desktop

package main

import (
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	assets "novel-assistant"
	"novel-assistant/desktop"
)

func main() {
	// assets.FS embeds the entire web/ directory tree.
	// fs.Sub strips the "web" prefix so the sub-FS root is web/.
	webFS, err := fs.Sub(assets.FS, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	app, err := desktop.New(webFS)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}

	if err := wails.Run(&options.App{
		Title:     "小說助手",
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			// gin is the asset handler — the WebView fetches all pages from gin
			// directly without opening a TCP port.
			Handler: app.Handler(),
		},
		OnStartup: app.Startup,
		Bind:      []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	}); err != nil {
		log.Fatalf("wails: %v", err)
	}
}
