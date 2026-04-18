package server

import (
	"novel-assistant/internal/config"
	"testing"
)

func TestNormalizeChapterName(t *testing.T) {
	t.Parallel()

	name, err := normalizeChapterName("第03章_雨夜對峙")
	if err != nil {
		t.Fatalf("expected valid chapter name, got error: %v", err)
	}
	if name != "第03章_雨夜對峙.md" {
		t.Fatalf("unexpected normalized name: %s", name)
	}
}

func TestNormalizeChapterNameRejectsTraversal(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"../secret.md", "folder/chapter.md", "folder\\chapter.md", ""} {
		if _, err := normalizeChapterName(raw); err == nil {
			t.Fatalf("expected invalid chapter name for %q", raw)
		}
	}
}

func TestSaveAndLoadChapterFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg: &config.Config{DataDir: dir},
	}

	saved, err := s.saveChapterFile("第01章_開場", "林昊推門而入。")
	if err != nil {
		t.Fatalf("expected save to succeed, got error: %v", err)
	}
	if saved.Name != "第01章_開場.md" {
		t.Fatalf("unexpected saved filename: %s", saved.Name)
	}

	loaded, err := s.loadChapterFile(saved.Name)
	if err != nil {
		t.Fatalf("expected load to succeed, got error: %v", err)
	}
	if loaded.Content != "林昊推門而入。" {
		t.Fatalf("unexpected loaded content: %s", loaded.Content)
	}
}
