package vectorstore

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"sync"
)

type Document struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "character" | "world"
	Content   string    `json:"content"`
	Embedding []float64 `json:"embedding"`
}

type Store struct {
	mu       sync.RWMutex
	docs     []Document
	filepath string
}

func New(filepath string) *Store {
	return &Store{filepath: filepath}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.filepath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.docs)
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.docs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filepath, data, 0644)
}

func (s *Store) Upsert(doc Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.docs {
		if d.ID == doc.ID {
			s.docs[i] = doc
			return
		}
	}
	s.docs = append(s.docs, doc)
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = nil
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.docs)
}

func (s *Store) Query(queryVec []float64, topK int, docType string) []Document {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		doc   Document
		score float64
	}
	var results []scored
	for _, d := range s.docs {
		if docType != "" && d.Type != docType {
			continue
		}
		results = append(results, scored{d, cosine(queryVec, d.Embedding)})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	out := make([]Document, 0, topK)
	for i := 0; i < topK && i < len(results); i++ {
		out = append(out, results[i].doc)
	}
	return out
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
