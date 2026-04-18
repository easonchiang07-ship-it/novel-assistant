package config

type Config struct {
	OllamaURL  string
	LLMModel   string
	EmbedModel string
	DataDir    string
	Port       string
}

func Default() *Config {
	return &Config{
		OllamaURL:  "http://localhost:11434",
		LLMModel:   "llama3.2",
		EmbedModel: "nomic-embed-text",
		DataDir:    "data",
		Port:       "8080",
	}
}
