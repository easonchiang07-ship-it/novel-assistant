package server

import (
	"context"
	"novel-assistant/internal/profile"
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
