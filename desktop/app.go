//go:build desktop

package desktop

import (
	"context"
	"io/fs"
	"log"
	"net/http"

	"novel-assistant/internal/config"
	"novel-assistant/internal/server"
)

// App is the Wails application struct. It holds the gin server which is
// mounted as the Wails AssetServer.Handler so the WebView renders gin pages.
type App struct {
	ctx context.Context
	srv *server.Server
}

// New creates an App and initializes the embedded gin server.
// Must be called before wails.Run so the Handler is ready for the options struct.
func New(webFS fs.FS) (*App, error) {
	cfg := config.Default()
	srv, err := server.NewEmbedded(cfg, webFS)
	if err != nil {
		return nil, err
	}
	return &App{srv: srv}, nil
}

// Startup is called by Wails after the window is created.
// Ingest runs in the background so the UI is immediately usable.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		if err := a.srv.Ingest(ctx); err != nil {
			log.Printf("desktop: ingest failed (Ollama may not be running): %v", err)
		}
	}()
}

// Handler returns the gin engine as an http.Handler for Wails AssetServer.
func (a *App) Handler() http.Handler {
	return a.srv.Handler()
}
