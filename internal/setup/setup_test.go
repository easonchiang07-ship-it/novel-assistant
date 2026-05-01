package setup

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Recommend ────────────────────────────────────────────────────────────────

func TestRecommend_LowRAM(t *testing.T) {
	rec := Recommend(SystemSpecs{RAMGB: 3.5})
	if rec.LLMModelName != "llama3.2:1b" {
		t.Errorf("< 4 GB RAM: want llama3.2:1b, got %s", rec.LLMModelName)
	}
	if rec.EmbedModelName != "nomic-embed-text" {
		t.Errorf("embed model: want nomic-embed-text, got %s", rec.EmbedModelName)
	}
}

func TestRecommend_MidRAM(t *testing.T) {
	rec := Recommend(SystemSpecs{RAMGB: 8})
	if rec.LLMModelName != "llama3.2" {
		t.Errorf("8 GB RAM: want llama3.2, got %s", rec.LLMModelName)
	}
}

func TestRecommend_HighRAM(t *testing.T) {
	rec := Recommend(SystemSpecs{RAMGB: 32})
	if rec.LLMModelName != "llama3.1:8b" {
		t.Errorf(">= 16 GB RAM: want llama3.1:8b, got %s", rec.LLMModelName)
	}
}

func TestRecommend_GPUVRAMOverridesRAM(t *testing.T) {
	// Low system RAM but high GPU VRAM → should recommend the high-quality model.
	rec := Recommend(SystemSpecs{RAMGB: 6, VRAMGB: 16})
	if rec.LLMModelName != "llama3.1:8b" {
		t.Errorf("VRAM 16 GB: want llama3.1:8b, got %s", rec.LLMModelName)
	}
}

func TestRecommend_NoteNonEmpty(t *testing.T) {
	specs := []SystemSpecs{
		{RAMGB: 2},
		{RAMGB: 8},
		{RAMGB: 32},
	}
	for _, s := range specs {
		rec := Recommend(s)
		if rec.Note == "" {
			t.Errorf("Recommend(%+v): Note should not be empty", s)
		}
	}
}

// ── IsAllowedModel ───────────────────────────────────────────────────────────

func TestIsAllowedModel_KnownLLM(t *testing.T) {
	known := []string{"llama3.2:1b", "phi3:mini", "llama3.2", "llama3.1:8b", "mistral"}
	for _, name := range known {
		if !IsAllowedModel(name) {
			t.Errorf("IsAllowedModel(%q): want true", name)
		}
	}
}

func TestIsAllowedModel_KnownEmbed(t *testing.T) {
	if !IsAllowedModel("nomic-embed-text") {
		t.Error("IsAllowedModel(nomic-embed-text): want true")
	}
}

func TestIsAllowedModel_Unknown(t *testing.T) {
	unknown := []string{"", "evil-model", "llama3.2:latest", "../etc/passwd"}
	for _, name := range unknown {
		if IsAllowedModel(name) {
			t.Errorf("IsAllowedModel(%q): want false", name)
		}
	}
}

// ── MarkComplete / IsComplete ────────────────────────────────────────────────

func TestMarkComplete_CreatesMarker(t *testing.T) {
	dir := t.TempDir()
	if IsComplete(dir) {
		t.Fatal("IsComplete: want false before MarkComplete")
	}
	if err := MarkComplete(dir); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	if !IsComplete(dir) {
		t.Error("IsComplete: want true after MarkComplete")
	}
	if _, err := os.Stat(filepath.Join(dir, ".setup_complete")); err != nil {
		t.Errorf("marker file not found: %v", err)
	}
}

func TestMarkComplete_IdempotentOnRepeat(t *testing.T) {
	dir := t.TempDir()
	if err := MarkComplete(dir); err != nil {
		t.Fatalf("first MarkComplete: %v", err)
	}
	if err := MarkComplete(dir); err != nil {
		t.Fatalf("second MarkComplete: %v", err)
	}
	if !IsComplete(dir) {
		t.Error("IsComplete: want true after two MarkComplete calls")
	}
}

func TestIsComplete_MissingDir(t *testing.T) {
	if IsComplete(filepath.Join(t.TempDir(), "nonexistent")) {
		t.Error("IsComplete on missing dir: want false")
	}
}
