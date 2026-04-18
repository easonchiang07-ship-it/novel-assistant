package reviewrules

import "testing"

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
