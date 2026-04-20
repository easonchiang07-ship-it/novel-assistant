package config

import (
	"os"
	"strings"
)

type Config struct {
	OllamaURL        string
	LLMModel         string
	EmbedModel       string
	DataDir          string
	Port             string
	AuthMode         string
	AuthPassword     string
	AuthCookieSecure bool
}

func Default() *Config {
	cfg := &Config{
		OllamaURL:  "http://localhost:11434",
		LLMModel:   "llama3.2",
		EmbedModel: "nomic-embed-text",
		DataDir:    "data",
		Port:       "8080",
		AuthMode:   "open",
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
	if v := os.Getenv("AUTH_MODE"); v != "" {
		cfg.AuthMode = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("AUTH_PASSWORD"); v != "" {
		cfg.AuthPassword = v
		if cfg.AuthMode == "open" {
			cfg.AuthMode = "password"
		}
	}
	if v := os.Getenv("AUTH_COOKIE_SECURE"); v != "" {
		cfg.AuthCookieSecure = parseBool(v)
	}
	return cfg
}

func (c *Config) AuthEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(c.AuthMode), "password") && strings.TrimSpace(c.AuthPassword) != ""
}

func (c *Config) GetAuthPassword() string {
	return c.AuthPassword
}

func (c *Config) GetAuthCookieSecure() bool {
	return c.AuthCookieSecure
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
