package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/browser"

	assets "novel-assistant"
	"novel-assistant/internal/config"
	"novel-assistant/internal/server"
	"novel-assistant/internal/setup"
)

func main() {
	loadDotEnv(".env")
	gin.SetMode(gin.ReleaseMode)
	cfg := config.Default()

	// Read the global data directory BEFORE server.New, which calls
	// setProjectState and mutates cfg.DataDir to the active project dir.
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "data"
	}

	// Serve web assets from the embedded FS so the binary is self-contained
	// and works from any directory (no web/ folder required at runtime).
	webFS, err := fs.Sub(assets.FS, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	srv, err := server.NewEmbedded(cfg, webFS)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	firstRun := !setup.IsComplete(dataDir)
	url := "http://localhost:" + cfg.Port
	if firstRun {
		url += "/setup"
	}

	fmt.Printf("小說助手啟動中... %s\n", url)

	// Run Ingest in the background so it does not block the server from
	// accepting connections. On first run Ollama is not yet installed, so
	// we skip it entirely.
	if !firstRun {
		go func() {
			fmt.Println("正在建立向量索引（背景執行中）...")
			ctx := context.Background()
			if err := srv.Ingest(ctx); err != nil {
				log.Printf("向量索引失敗（若 Ollama 未啟動可忽略）: %v", err)
			} else {
				log.Println("向量索引完成")
			}
		}()
	}

	// Open the browser after the server has had a moment to start listening.
	// Because Ingest is now non-blocking, srv.Run() is reached almost
	// immediately, so 500 ms is a safe margin.
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := browser.OpenURL(url); err != nil {
			log.Printf("無法自動開啟瀏覽器，請手動前往 %s", url)
		}
	}()

	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
