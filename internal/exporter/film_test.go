package exporter

import (
	"strings"
	"testing"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/profile"
)

func TestExportFilmScenes_YAML(t *testing.T) {
	scenes := []checker.FilmScene{
		{
			SceneID:  "CH01-S01",
			Location: "台北雨夜巷弄",
			Time:     "夜晚",
			Mood:     "緊繃",
			Characters: []checker.FilmCharacter{
				{Name: "志明", Action: "握緊拳頭", Emotion: "壓抑"},
			},
			Dialogue: []checker.FilmLine{
				{Speaker: "志明", Text: "這債，我會還完。"},
			},
			VideoPrompt: "A tired man stands in a rain-soaked alley.",
		},
	}

	out, err := ExportFilmScenes(scenes, FilmFormatYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "CH01-S01") {
		t.Error("expected scene_id in YAML output")
	}
	if !strings.Contains(body, "video_prompt") {
		t.Error("expected video_prompt in YAML output")
	}
}

func TestExportFilmScenes_JSON(t *testing.T) {
	scenes := []checker.FilmScene{
		{SceneID: "CH01-S01", Location: "台北", VideoPrompt: "A man walks."},
	}

	out, err := ExportFilmScenes(scenes, FilmFormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, `"scene_id"`) {
		t.Error("expected scene_id in JSON output")
	}
	if !strings.Contains(body, `"video_prompt"`) {
		t.Error("expected video_prompt in JSON output")
	}
}

func TestInjectVisualGuard_FillsAppearanceAndPrependsPrompt(t *testing.T) {
	t.Parallel()

	scenes := []checker.FilmScene{
		{
			SceneID:     "CH01-S01",
			VideoPrompt: "A tired man stands in a rain-soaked alley.",
			Characters: []checker.FilmCharacter{
				{Name: "林逸", Action: "低下頭"},
			},
		},
	}
	characters := []*profile.Character{
		{Name: "林逸", Appearance: "黑色長外套；黑框眼鏡"},
	}

	out := InjectVisualGuard(scenes, characters)

	if out[0].Characters[0].Appearance != "黑色長外套；黑框眼鏡" {
		t.Fatalf("expected appearance injected into character, got %q", out[0].Characters[0].Appearance)
	}
	if !strings.HasPrefix(out[0].VideoPrompt, "[林逸: 黑色長外套；黑框眼鏡]") {
		t.Fatalf("expected appearance anchor prepended to video_prompt, got %q", out[0].VideoPrompt)
	}
	if !strings.Contains(out[0].VideoPrompt, "A tired man") {
		t.Fatalf("expected original prompt preserved, got %q", out[0].VideoPrompt)
	}
}

func TestInjectVisualGuard_SkipsCharacterWithoutAppearance(t *testing.T) {
	t.Parallel()

	scenes := []checker.FilmScene{
		{
			SceneID:     "CH01-S01",
			VideoPrompt: "A woman walks alone.",
			Characters:  []checker.FilmCharacter{{Name: "陳靜", Action: "走路"}},
		},
	}
	characters := []*profile.Character{
		{Name: "陳靜", Appearance: ""},
	}

	out := InjectVisualGuard(scenes, characters)

	if out[0].Characters[0].Appearance != "" {
		t.Fatalf("expected empty appearance unchanged, got %q", out[0].Characters[0].Appearance)
	}
	if out[0].VideoPrompt != "A woman walks alone." {
		t.Fatalf("expected video_prompt unchanged when no appearance, got %q", out[0].VideoPrompt)
	}
}

func TestInjectVisualGuard_NoProfilesReturnsOriginal(t *testing.T) {
	t.Parallel()

	scenes := []checker.FilmScene{
		{SceneID: "S01", VideoPrompt: "A man runs.", Characters: []checker.FilmCharacter{{Name: "志明"}}},
	}

	out := InjectVisualGuard(scenes, nil)

	if out[0].VideoPrompt != "A man runs." {
		t.Fatalf("expected original scenes returned unchanged, got %q", out[0].VideoPrompt)
	}
}

func TestExportFilmScenes_DefaultsToYAML(t *testing.T) {
	scenes := []checker.FilmScene{{SceneID: "S01"}}
	out, err := ExportFilmScenes(scenes, "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// YAML does not use double-quoted keys like JSON
	if strings.Contains(string(out), `"scene_id"`) {
		t.Error("expected YAML format, got JSON-style output")
	}
}
