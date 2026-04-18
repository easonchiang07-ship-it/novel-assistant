package main

import (
	"context"
	"fmt"
	"log"
	"novel-assistant/internal/config"
	"novel-assistant/internal/server"
)

func main() {
	loadDotEnv(".env")
	cfg := config.Default()

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	fmt.Printf("小說助手啟動中... http://localhost:%s\n", cfg.Port)
	fmt.Println("正在建立向量索引（需要 Ollama 執行中）...")

	ctx := context.Background()
	if err := srv.Ingest(ctx); err != nil {
		log.Printf("向量索引失敗（若 Ollama 未啟動可忽略）: %v", err)
	} else {
		fmt.Println("索引完成，開始服務...")
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
