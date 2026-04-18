package config

import "os"

type Config struct {
	OllamaURL  string
	LLMModel   string
	EmbedModel string
	DataDir    string
	Port       string
}

func Default() *Config {
	cfg := &Config{
		OllamaURL:  "http://localhost:11434",
		LLMModel:   "llama3.2",
		EmbedModel: "nomic-embed-text",
		DataDir:    "data",
		Port:       "8080",
	}
	if v := os.Getenv("OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLMModel = v
	}
	if v := os.Getenv("EMBED_MODEL"); v != "" {
		cfg.EmbedModel = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	return cfg
}
