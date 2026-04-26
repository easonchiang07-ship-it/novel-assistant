package exporter

import (
	"strings"
	"testing"

	"novel-assistant/internal/checker"
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
