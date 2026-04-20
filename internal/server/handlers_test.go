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

	refs, err := s.buildReferenceContext(context.Background(), "chapter", "", retrievalOptions{})
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

func TestCheckRequestRetrievalOverrideForTask(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Retrieval: retrievalOptions{
			Sources:   []string{"character"},
			TopK:      4,
			Threshold: 0.1,
		},
		RetrievalOverrides: map[string]retrievalOptions{
			"world": {
				Sources:      []string{"world"},
				TopK:         2,
				Threshold:    0,
				ThresholdSet: true,
			},
		},
	}

	world := req.retrievalOverrideFor("world")
	if len(world.Sources) != 1 || world.Sources[0] != "world" || world.TopK != 2 || !world.ThresholdSet {
		t.Fatalf("expected task-specific override, got %#v", world)
	}

	behavior := req.retrievalOverrideFor("behavior")
	if len(behavior.Sources) != 1 || behavior.Sources[0] != "character" || behavior.TopK != 4 {
		t.Fatalf("expected fallback to shared retrieval, got %#v", behavior)
	}
}

func TestComputeRetrievalGapsReportsOnlyUnretrievedSignals(t *testing.T) {
	t.Parallel()

	chapter := "林昊走進夜港塔下。影潮契約已經啟動。"
	retrieved := []vectorProfile{
		{Name: "林昊", Type: "character"},
	}

	gaps := computeRetrievalGaps(chapter, []string{"林昊", "白璃"}, retrieved)

	if len(gaps.MissingCharacters) != 0 {
		t.Fatalf("expected retrieved known characters to be excluded, got %#v", gaps.MissingCharacters)
	}
	if len(gaps.MissingLocations) != 1 || gaps.MissingLocations[0] != "夜港塔下" {
		t.Fatalf("expected missing location to be reported, got %#v", gaps.MissingLocations)
	}
	if len(gaps.MissingSettings) != 1 || gaps.MissingSettings[0] != "影潮契約" {
		t.Fatalf("expected missing setting to be reported, got %#v", gaps.MissingSettings)
	}
}

func TestResolveCharactersIncludesPronounCandidates(t *testing.T) {
	t.Parallel()

	s := &Server{
		profiles: &profile.Manager{
			Characters: []*profile.Character{
				{Name: "林昊", RawContent: "# 角色：林昊\n- 個性：冷靜\n- 性別：男性"},
				{Name: "白璃", RawContent: "# 角色：白璃\n- 個性：果斷\n- 性別：女性"},
			},
		},
	}

	chars := s.resolveCharacters(checkRequest{
		Chapter: "林昊看著她，沒有立刻回答。",
		Checks:  []string{"behavior"},
	})
	if len(chars) != 2 {
		t.Fatalf("expected explicit and pronoun candidates, got %#v", chars)
	}
	if chars[0].Name != "林昊" || chars[1].Name != "白璃" {
		t.Fatalf("unexpected resolved characters: %#v", chars)
	}
}
