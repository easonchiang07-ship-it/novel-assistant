package server

import (
	"context"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/vectorstore"
	"testing"
)

func TestParsePositiveChapter(t *testing.T) {
	t.Parallel()

	chapter, err := parsePositiveChapter("12")
	if err != nil {
		t.Fatalf("expected valid chapter, got error: %v", err)
	}
	if chapter != 12 {
		t.Fatalf("expected 12, got %d", chapter)
	}
}

func TestParsePositiveChapterRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "0", "-1", "abc"} {
		if _, err := parsePositiveChapter(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestResolveStylesReturnsAllStylesWhenNoneSelected(t *testing.T) {
	t.Parallel()

	s := &Server{
		profiles: &profile.Manager{
			Styles: []*profile.StyleGuide{
				{Name: "主線敘事", RawContent: "# 風格：主線敘事"},
				{Name: "回憶場景", RawContent: "# 風格：回憶場景"},
			},
		},
	}

	styles, err := s.resolveStyles(checkRequest{
		Checks: []string{"style"},
	})
	if err != nil {
		t.Fatalf("expected styles resolved, got error: %v", err)
	}
	if len(styles) != 2 {
		t.Fatalf("expected 2 styles, got %d", len(styles))
	}
}

func TestResolveStylesRejectsMissingStyle(t *testing.T) {
	t.Parallel()

	s := &Server{
		profiles: &profile.Manager{
			Styles: []*profile.StyleGuide{
				{Name: "主線敘事", RawContent: "# 風格：主線敘事"},
			},
		},
	}

	_, err := s.resolveStyles(checkRequest{
		Checks: []string{"style"},
		Styles: []string{"不存在"},
	})
	if err == nil {
		t.Fatal("expected error for missing style")
	}
}

func TestResolveStylesRejectsEmptyContent(t *testing.T) {
	t.Parallel()

	s := &Server{
		profiles: &profile.Manager{
			Styles: []*profile.StyleGuide{
				{Name: "空白風格", RawContent: "   "},
			},
		},
	}

	_, err := s.resolveStyles(checkRequest{
		Checks: []string{"style"},
		Styles: []string{"空白風格"},
	})
	if err == nil {
		t.Fatal("expected error for empty style content")
	}
}

func TestRewriteInstructionRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	if _, err := rewriteInstruction("unknown"); err == nil {
		t.Fatal("expected error for unknown rewrite mode")
	}
}

func TestBuildReferenceContextReturnsNilWhenStoreIsEmpty(t *testing.T) {
	t.Parallel()

	s := &Server{
		store: &vectorstore.Store{},
	}

	refs, err := s.buildReferenceContext(context.Background(), "chapter", retrievalOptions{})
	if err != nil {
		t.Fatalf("unexpected error with empty store: %v", err)
	}
	if refs != nil {
		t.Fatalf("expected nil refs for empty store, got %#v", refs)
	}
}

func TestMergeRetrievalUsesPresetUntilOverrideProvided(t *testing.T) {
	t.Parallel()

	preset := reviewrules.RetrievalPreset{
		Sources:   []string{"character", "world"},
		TopK:      4,
		Threshold: 0.25,
	}

	got := mergeRetrieval(preset, retrievalOptions{})
	if len(got.Sources) != 2 || got.TopK != 4 || got.Threshold != 0.25 {
		t.Fatalf("expected preset values to survive zero-value override, got %#v", got)
	}

	got = mergeRetrieval(preset, retrievalOptions{
		Sources:      []string{"style"},
		TopK:         2,
		Threshold:    0.8,
		ThresholdSet: true,
	})
	if len(got.Sources) != 1 || got.Sources[0] != "style" {
		t.Fatalf("expected sources override, got %#v", got.Sources)
	}
	if got.TopK != 2 || got.Threshold != 0.8 {
		t.Fatalf("expected numeric overrides, got %#v", got)
	}
}

func TestMergeRetrievalAllowsThresholdOverrideToZero(t *testing.T) {
	t.Parallel()

	preset := reviewrules.RetrievalPreset{
		Sources:   []string{"world"},
		TopK:      4,
		Threshold: 0.6,
	}

	got := mergeRetrieval(preset, retrievalOptions{
		Threshold:    0,
		ThresholdSet: true,
	})
	if got.Threshold != 0 {
		t.Fatalf("expected threshold override to zero, got %#v", got)
	}
}
