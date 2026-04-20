package projectsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	OllamaURL       string `json:"ollama_url"`
	LLMModel        string `json:"llm_model"`
	EmbedModel      string `json:"embed_model"`
	Port            string `json:"port"`
	DataDir         string `json:"data_dir"`
	BackupRetention int    `json:"backup_retention"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	item Settings
}

func Defaults() Settings {
	return Settings{
		OllamaURL:       "http://localhost:11434",
		LLMModel:        "llama3.2",
		EmbedModel:      "nomic-embed-text",
		Port:            "8080",
		DataDir:         "data",
		BackupRetention: 10,
	}
}

func New(path string, base Settings) *Store {
	base = normalize(base)
	return &Store{path: path, item: base}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.item = normalize(s.item)
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s.item); err != nil {
		return err
	}
	s.item = normalize(s.item)
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.item
}

func (s *Store) Update(item Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.item = normalize(item)
}

func normalize(item Settings) Settings {
	def := Defaults()
	if item.OllamaURL == "" {
		item.OllamaURL = def.OllamaURL
	}
	if item.LLMModel == "" {
		item.LLMModel = def.LLMModel
	}
	if item.EmbedModel == "" {
		item.EmbedModel = def.EmbedModel
	}
	if item.Port == "" {
		item.Port = def.Port
	}
	if item.DataDir == "" {
		item.DataDir = def.DataDir
	}
	if item.BackupRetention < 1 {
		item.BackupRetention = def.BackupRetention
	}
	return item
}
