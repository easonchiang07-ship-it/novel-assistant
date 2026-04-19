package reviewrules

import (
	"path/filepath"
	"testing"
)

func TestNormalizeAppliesDefaults(t *testing.T) {
	t.Parallel()

	item := normalize(Settings{})
	if len(item.DefaultChecks) == 0 || item.ReviewBias == "" || item.RewriteBias == "" {
		t.Fatalf("expected defaults, got %#v", item)
	}
}

func TestNormalizeDeduplicatesDefaults(t *testing.T) {
	t.Parallel()

	item := normalize(Settings{
		DefaultChecks: []string{"behavior", "behavior", "world"},
		DefaultStyles: []string{"主線", "主線"},
		ReviewBias:    "strict",
		RewriteBias:   "expressive",
	})
	if len(item.DefaultChecks) != 2 {
		t.Fatalf("expected deduped checks, got %#v", item.DefaultChecks)
	}
	if len(item.DefaultStyles) != 1 {
		t.Fatalf("expected deduped styles, got %#v", item.DefaultStyles)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Parallel()

	store := New(filepath.Join(t.TempDir(), "review_rules.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load missing settings file: %v", err)
	}

	got := store.Get()
	if got.RetrievalTopK != 4 || got.RetrievalThreshold != 0 {
		t.Fatalf("expected retrieval defaults, got %#v", got)
	}
	if len(got.RetrievalSources) != 3 {
		t.Fatalf("expected default retrieval sources, got %#v", got.RetrievalSources)
	}
}

func TestNormalizeRetrievalSettings(t *testing.T) {
	t.Parallel()

	item := normalize(Settings{
		RetrievalSources:   []string{"character", "unknown", "character"},
		RetrievalTopK:      99,
		RetrievalThreshold: -0.5,
	})

	if len(item.RetrievalSources) != 1 || item.RetrievalSources[0] != "character" {
		t.Fatalf("expected filtered retrieval sources, got %#v", item.RetrievalSources)
	}
	if item.RetrievalTopK != Defaults().RetrievalTopK {
		t.Fatalf("expected retrieval top-k fallback, got %d", item.RetrievalTopK)
	}
	if item.RetrievalThreshold != 0 {
		t.Fatalf("expected retrieval threshold fallback, got %v", item.RetrievalThreshold)
	}
}

func TestNormalizeRetrievalSourcesFallbackToDefaults(t *testing.T) {
	t.Parallel()

	item := normalize(Settings{
		RetrievalSources: []string{"unknown"},
	})

	if len(item.RetrievalSources) != len(Defaults().RetrievalSources) {
		t.Fatalf("expected default retrieval sources, got %#v", item.RetrievalSources)
	}
}
