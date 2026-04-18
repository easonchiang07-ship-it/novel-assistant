package server

import (
	"path/filepath"
	"testing"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/tracker"
)

func TestOrderedChapterFilesRespectsSavedOrder(t *testing.T) {
	t.Parallel()

	files := []chapterFile{
		{Name: "第02章.md"},
		{Name: "第01章.md"},
		{Name: "第03章.md"},
	}
	order := []string{"第03章.md", "第01章.md"}
	got := orderedChapterFiles(files, order)
	if got[0].Name != "第03章.md" || got[1].Name != "第01章.md" || got[2].Name != "第02章.md" {
		t.Fatalf("unexpected ordered chapter files: %#v", got)
	}
}

func TestOrderedScenesRespectsSavedOrder(t *testing.T) {
	t.Parallel()

	scenes := []Scene{
		{Index: 1, Title: "Scene 1: Opening"},
		{Index: 2, Title: "Scene 2: Rain"},
		{Index: 3, Title: "Scene 3: Rooftop"},
	}
	got := orderedScenes(scenes, []string{"Scene 3: Rooftop", "Scene 1: Opening"})
	if got[0].Title != "Scene 3: Rooftop" || got[1].Title != "Scene 1: Opening" || got[2].Title != "Scene 2: Rain" {
		t.Fatalf("unexpected ordered scenes: %#v", got)
	}
}

func TestRebuildChapterWithSceneOrderPreservesPreamble(t *testing.T) {
	t.Parallel()

	content := `前言

## Scene 1: Opening
Open.

## Scene 2: Rain
Rain.`
	scenes := parseScenes(content)
	got := rebuildChapterWithSceneOrder(content, scenes, []string{"Scene 2: Rain", "Scene 1: Opening"})
	want := "前言\n\n## Scene 2: Rain\nRain.\n\n## Scene 1: Opening\nOpen."
	if got != want {
		t.Fatalf("unexpected rebuilt chapter:\n%s", got)
	}
}

func TestListChapterFilesUsesSavedOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{cfg: &config.Config{DataDir: dir}}
	if _, err := s.saveChapterFile("第02章", "b"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	if _, err := s.saveChapterFile("第01章", "a"); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if err := s.saveChapterOrder([]string{"第02章.md", "第01章.md"}); err != nil {
		t.Fatalf("save chapter order: %v", err)
	}

	files, err := s.listChapterFiles()
	if err != nil {
		t.Fatalf("list chapter files: %v", err)
	}
	if len(files) != 2 || files[0].Name != "第02章.md" || files[1].Name != "第01章.md" {
		t.Fatalf("unexpected chapter file order: %#v", files)
	}
}

func TestBuildChapterOverviewsPreservesSavedOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		history:    reviewhistory.New(filepath.Join(dir, "reviews.json")),
		timeline:   tracker.NewTimelineTracker(filepath.Join(dir, "timeline.json")),
		foreshadow: tracker.NewForeshadowTracker(filepath.Join(dir, "foreshadow.json")),
	}
	if _, err := s.saveChapterFile("第02章", "b"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	if _, err := s.saveChapterFile("第01章", "a"); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if err := s.saveChapterOrder([]string{"第02章.md", "第01章.md"}); err != nil {
		t.Fatalf("save chapter order: %v", err)
	}

	overviews, err := s.buildChapterOverviews()
	if err != nil {
		t.Fatalf("buildChapterOverviews: %v", err)
	}
	if len(overviews) != 2 || overviews[0].Name != "第02章.md" || overviews[1].Name != "第01章.md" {
		t.Fatalf("buildChapterOverviews did not preserve saved order: %v", overviewNames(overviews))
	}
}

func overviewNames(overviews []chapterOverview) []string {
	names := make([]string, len(overviews))
	for i, o := range overviews {
		names[i] = o.Name
	}
	return names
}
