package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/vectorstore"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestDefaultReviewLayersReturnsFourLayersInOrder(t *testing.T) {
	t.Parallel()

	layers := defaultReviewLayers()
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers, got %d", len(layers))
	}

	want := []struct {
		name  string
		label string
	}{
		{name: "structure", label: "結構層"},
		{name: "character", label: "角色層"},
		{name: "world_logic", label: "世界觀層"},
		{name: "language", label: "語言層"},
	}

	for i, layer := range layers {
		if layer.Name != want[i].name || layer.Label != want[i].label || !layer.Enabled {
			t.Fatalf("unexpected layer at %d: %#v", i, layer)
		}
		if strings.TrimSpace(layer.Prompt) == "" {
			t.Fatalf("expected prompt for layer %s", layer.Name)
		}
	}
}

func TestResolveReviewLayersPipelineReturnsAllEnabled(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter:   "章節內容",
		LayerMode: "pipeline",
	}

	layers := resolveReviewLayers(req)
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers in pipeline mode, got %#v", layers)
	}
}

func TestResolveReviewLayersSingleReturnsNil(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter:   "章節內容",
		LayerMode: "single",
	}

	layers := resolveReviewLayers(req)
	if layers != nil {
		t.Fatalf("expected nil layers in single mode, got %#v", layers)
	}
}

func TestResolveReviewLayersTreatsEmptyModeAsSingle(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter:   "章節內容",
		LayerMode: normalizedLayerMode("   "),
	}

	layers := resolveReviewLayers(req)
	if layers != nil {
		t.Fatalf("expected nil layers for empty mode, got %#v", layers)
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

func TestStylePresetInstructionReturnsConstraintText(t *testing.T) {
	t.Parallel()

	text, err := stylePresetInstruction("cold_hard")
	if err != nil {
		t.Fatalf("expected preset to resolve, got error: %v", err)
	}
	if !strings.Contains(text, "冷硬派") {
		t.Fatalf("expected preset text to mention cold_hard theme, got %q", text)
	}
}

func TestStylePresetInstructionRejectsUnknownPreset(t *testing.T) {
	t.Parallel()

	if _, err := stylePresetInstruction("unknown"); err == nil {
		t.Fatal("expected unknown preset error")
	}
}

func TestSaveStyleAnalysisWritesSidecarJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stylePath := filepath.Join(dir, "style", "主線敘事.md")
	if err := os.MkdirAll(filepath.Dir(stylePath), 0755); err != nil {
		t.Fatalf("mkdir style dir: %v", err)
	}
	if err := os.WriteFile(stylePath, []byte("# 風格：主線敘事"), 0644); err != nil {
		t.Fatalf("write style file: %v", err)
	}

	s := &Server{}
	analysis := profile.StyleAnalysis{
		DialogueRatio:  "低",
		SensoryFreq:    "中",
		AvgSentenceLen: "短促",
		Tone:           "冷靜",
		Summary:        "節制克制",
	}
	if err := s.saveStyleAnalysis(stylePath, analysis); err != nil {
		t.Fatalf("save style analysis: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "style", ".analysis", "主線敘事.json"))
	if err != nil {
		t.Fatalf("read analysis file: %v", err)
	}
	var got profile.StyleAnalysis
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode analysis file: %v", err)
	}
	if got != analysis {
		t.Fatalf("unexpected saved analysis: got %#v want %#v", got, analysis)
	}
}

func TestHandleApplyStyleAnalysisReturnsNotFoundForMissingStyle(t *testing.T) {
	t.Parallel()

	s := &Server{
		profiles: &profile.Manager{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/styles/%E4%B8%8D%E5%AD%98%E5%9C%A8/analysis", strings.NewReader(`{"dialogue_ratio":"高"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router = ginTestRouter(s)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing style, got %d", w.Code)
	}
}

func ginTestRouter(s *Server) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/styles/:name/analysis", s.handleApplyStyleAnalysis)
	return r
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

func TestSummarizeReferencesIncludesChunkMetadata(t *testing.T) {
	t.Parallel()

	items := []vectorProfile{
		{
			Name:         "第02章_scene_1",
			Type:         "chapter",
			Content:      "林昊推門走進雨夜碼頭。",
			Score:        0.91,
			MatchReason:  "章節直接提到此參考名稱",
			ChapterMatch: "…雨夜碼頭…",
			ChapterFile:  "第02章.md",
			ChapterIndex: 2,
			SceneIndex:   1,
			ChunkType:    "scene",
		},
	}

	got := summarizeReferences(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 summary, got %#v", got)
	}
	if got[0].ChapterFile != "第02章.md" || got[0].ChapterIndex != 2 || got[0].SceneIndex != 1 || got[0].ChunkType != "scene" {
		t.Fatalf("expected chunk metadata in summary, got %#v", got[0])
	}
}
