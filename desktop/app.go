//go:build desktop

package desktop

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel-assistant/internal/config"
	"novel-assistant/internal/server"
)

// App is the Wails application struct. It holds the gin server which is
// mounted as the Wails AssetServer.Handler so the WebView renders gin pages.
type App struct {
	ctx context.Context
	srv *server.Server
	cfg *config.Config
}

// New creates an App and initializes the embedded gin server.
// Must be called before wails.Run so the Handler is ready for the options struct.
func New(webFS fs.FS) (*App, error) {
	cfg := config.Default()

	// Desktop: store data in ~/NovelAssistant/data/ instead of ./data/
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	cfg.DataDir = filepath.Join(home, "NovelAssistant", "data")
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Printf("desktop: create data dir: %v", err)
	}

	srv, err := server.NewEmbedded(cfg, webFS)
	if err != nil {
		return nil, err
	}
	return &App{srv: srv, cfg: cfg}, nil
}

// Startup is called by Wails after the window is created.
// Checks Ollama reachability and navigates to /setup if not running.
// Ingest runs in the background so the UI is immediately usable once ready.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	go func() {
		// Short delay lets the WebView finish its initial load before we navigate.
		time.Sleep(400 * time.Millisecond)
		if !server.OllamaRunning(a.cfg.OllamaURL) {
			wailsruntime.WindowExecJS(ctx, `window.location.href='/setup';`)
			return
		}
		// Ollama is up — start indexing in the background.
		if err := a.srv.Ingest(ctx); err != nil {
			log.Printf("desktop: ingest failed: %v", err)
		}
	}()
}

// Handler returns the gin engine as an http.Handler for Wails AssetServer.
func (a *App) Handler() http.Handler {
	return a.srv.Handler()
}
