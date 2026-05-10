package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
)

// IndexManifest tracks per-file SHA-256 hashes to detect changes for incremental indexing.
type IndexManifest struct {
	Files map[string]string `json:"files"` // key -> SHA-256 hex
}

func newManifest() *IndexManifest {
	return &IndexManifest{Files: make(map[string]string)}
}

func loadManifest(path string) (*IndexManifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from server config, not user input
	if os.IsNotExist(err) {
		return newManifest(), nil
	}
	if err != nil {
		return nil, err
	}
	m := newManifest()
	if err := json.Unmarshal(data, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *IndexManifest) save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *IndexManifest) unchanged(key, content string) bool {
	return m.Files[key] == sha256Hex([]byte(content))
}

func (m *IndexManifest) record(key, content string) {
	m.Files[key] = sha256Hex([]byte(content))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
