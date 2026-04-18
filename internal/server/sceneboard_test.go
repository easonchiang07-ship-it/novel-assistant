package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/tracker"
)

func TestScenePlanRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		history:    reviewhistory.New(dir + "/reviews.json"),
		timeline:   tracker.NewTimelineTracker(dir + "/timeline.json"),
		foreshadow: tracker.NewForeshadowTracker(dir + "/foreshadow.json"),
	}

	if _, err := s.saveChapterFile("第01章", `## Scene 1: Opening
Lin Hao opened the door.`); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	err := s.saveScenePlan("第01章.md", scenePlan{
		Title:    "Scene 1: Opening",
		Synopsis: "Lin Hao enters the room.",
		POV:      "Lin Hao",
		Conflict: "He does not know who is waiting inside.",
		Purpose:  "Open the investigation thread.",
	})
	if err != nil {
		t.Fatalf("save scene plan: %v", err)
	}

	items, err := s.loadScenePlans("第01章.md")
	if err != nil {
		t.Fatalf("load scene plans: %v", err)
	}
	got, ok := items["Scene 1: Opening"]
	if !ok {
		t.Fatalf("expected scene plan to exist, got %v", items)
	}
	if got.POV != "Lin Hao" || got.Purpose != "Open the investigation thread." {
		t.Fatalf("unexpected round-trip scene plan: %#v", got)
	}
}

func TestBuildChapterOverviewsIncludesSceneBoardMetadataAndStatus(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		history:    reviewhistory.New(dir + "/reviews.json"),
		timeline:   tracker.NewTimelineTracker(dir + "/timeline.json"),
		foreshadow: tracker.NewForeshadowTracker(dir + "/foreshadow.json"),
	}

	_, err := s.saveChapterFile("第01章", `序章前言

## Scene 1: Opening
Lin Hao opened the door.

## Scene 2: Rain
Zhang Lei stood in the rain.`)
	if err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	if err := s.saveScenePlan("第01章.md", scenePlan{
		Title:    "Scene 1: Opening",
		Synopsis: "Lin Hao enters.",
		POV:      "Lin Hao",
		Conflict: "Unknown threat",
		Purpose:  "Start the chapter",
	}); err != nil {
		t.Fatalf("save scene 1 plan: %v", err)
	}

	s.history.Add(&reviewhistory.Entry{Kind: "review", ChapterFile: "第01章.md", SceneTitle: "Scene 1: Opening"})
	s.history.Add(&reviewhistory.Entry{Kind: "rewrite", ChapterFile: "第01章.md", SceneTitle: "Scene 2: Rain"})

	overviews, err := s.buildChapterOverviews()
	if err != nil {
		t.Fatalf("build chapter overviews: %v", err)
	}
	if len(overviews) != 1 {
		t.Fatalf("expected 1 chapter overview, got %d", len(overviews))
	}
	if overviews[0].SceneCount != 2 {
		t.Fatalf("expected 2 scenes, got %d", overviews[0].SceneCount)
	}
	if len(overviews[0].SceneCards) != 2 {
		t.Fatalf("expected 2 scene cards, got %d", len(overviews[0].SceneCards))
	}

	scene1 := overviews[0].SceneCards[0]
	scene2 := overviews[0].SceneCards[1]

	if scene1.Status != "reviewed" {
		t.Fatalf("expected scene 1 to be reviewed, got %q", scene1.Status)
	}
	if scene1.POV != "Lin Hao" || scene1.Synopsis != "Lin Hao enters." {
		t.Fatalf("expected scene 1 planning metadata, got %#v", scene1)
	}
	if scene2.Status != "rewritten" {
		t.Fatalf("expected scene 2 to be rewritten, got %q", scene2.Status)
	}
	if scene2.Preview == "" {
		t.Fatal("expected scene 2 preview to be populated")
	}
}

func TestBuildChapterOverviewsKeepsPlainChaptersWithoutSceneBoard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		history:    reviewhistory.New(dir + "/reviews.json"),
		timeline:   tracker.NewTimelineTracker(dir + "/timeline.json"),
		foreshadow: tracker.NewForeshadowTracker(dir + "/foreshadow.json"),
	}

	if _, err := s.saveChapterFile("第02章", "Lin Hao walked alone through the station."); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	overviews, err := s.buildChapterOverviews()
	if err != nil {
		t.Fatalf("build chapter overviews: %v", err)
	}
	if len(overviews) != 1 {
		t.Fatalf("expected 1 chapter overview, got %d", len(overviews))
	}
	if overviews[0].SceneCount != 0 {
		t.Fatalf("expected 0 scenes, got %d", overviews[0].SceneCount)
	}
	if len(overviews[0].SceneCards) != 0 {
		t.Fatalf("expected no scene cards, got %d", len(overviews[0].SceneCards))
	}
}

func TestBuildChapterOverviewsIgnoresCorruptScenePlanSidecar(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		history:    reviewhistory.New(dir + "/reviews.json"),
		timeline:   tracker.NewTimelineTracker(dir + "/timeline.json"),
		foreshadow: tracker.NewForeshadowTracker(dir + "/foreshadow.json"),
	}

	if _, err := s.saveChapterFile("第03章", `## Scene 1: Opening
Lin Hao opened the case file.`); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	sidecarPath, err := s.scenePlanPath("第03章.md")
	if err != nil {
		t.Fatalf("scenePlanPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sidecarPath), 0755); err != nil {
		t.Fatalf("mkdir sidecar dir: %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("write corrupt sidecar: %v", err)
	}

	overviews, err := s.buildChapterOverviews()
	if err != nil {
		t.Fatalf("expected corrupt sidecar to be ignored, got error: %v", err)
	}
	if len(overviews) != 1 || len(overviews[0].SceneCards) != 1 {
		t.Fatalf("expected one chapter with one scene card, got %#v", overviews)
	}
	scene := overviews[0].SceneCards[0]
	if strings.TrimSpace(scene.Synopsis) != "" || strings.TrimSpace(scene.POV) != "" {
		t.Fatalf("expected empty fallback scene plan metadata, got %#v", scene)
	}
}
