package main

import (
	"context"
	"fmt"
	"log"
	"novel-assistant/internal/config"
	"novel-assistant/internal/server"
	"novel-assistant/internal/setup"
	"time"

	"github.com/pkg/browser"
)

func main() {
	loadDotEnv(".env")
	cfg := config.Default()

	// Read the global data directory BEFORE server.New, which calls
	// setProjectState and mutates cfg.DataDir to the active project dir.
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "data"
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	firstRun := !setup.IsComplete(dataDir)
	url := "http://localhost:" + cfg.Port
	if firstRun {
		url += "/setup"
	}

	fmt.Printf("小說助手啟動中... %s\n", url)

	// Open the browser after a short delay so the server is ready to accept connections.
	go func() {
		time.Sleep(800 * time.Millisecond)
		if err := browser.OpenURL(url); err != nil {
			log.Printf("無法自動開啟瀏覽器，請手動前往 %s", url)
		}
	}()

	// Only build the vector index when setup has already been completed;
	// on the first run Ollama is not yet installed.
	if !firstRun {
		fmt.Println("正在建立向量索引（需要 Ollama 執行中）...")
		ctx := context.Background()
		if err := srv.Ingest(ctx); err != nil {
			log.Printf("向量索引失敗（若 Ollama 未啟動可忽略）: %v", err)
		} else {
			fmt.Println("索引完成，開始服務...")
		}
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
