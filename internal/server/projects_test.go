package server

import (
	"os"
	"path/filepath"
	"testing"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"
)

func TestSwitchProject_IsolatesChapterFiles(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	s := &Server{
		cfg: &config.Config{DataDir: dir},
		state: &projectState{
			dataDir:       filepath.Join(dir, "data"),
			profiles:      profile.NewManager(filepath.Join(dir, "data")),
			store:         vectorstore.New(filepath.Join(dir, "data", "store.json")),
			project:       projectsettings.New(filepath.Join(dir, "data", "project_settings.json"), projectsettings.Settings{DataDir: filepath.Join(dir, "data")}),
			rules:         reviewrules.New(filepath.Join(dir, "data", "review_rules.json")),
			history:       reviewhistory.New(filepath.Join(dir, "data", "reviews.json")),
			relationships: tracker.NewRelationshipTracker(filepath.Join(dir, "data", "relationships.json")),
			timeline:      tracker.NewTimelineTracker(filepath.Join(dir, "data", "timeline.json")),
			foreshadow:    tracker.NewForeshadowTracker(filepath.Join(dir, "data", "foreshadow.json")),
		},
	}
	s.setProjectState(s.state)

	if _, err := s.saveChapterFile("第01章", "default chapter"); err != nil {
		t.Fatalf("save default chapter: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("workspaces", "p2", "chapters"), 0755); err != nil {
		t.Fatalf("mkdir p2 chapters: %v", err)
	}
	if err := os.WriteFile(filepath.Join("workspaces", "p2", "chapters", "第02章.md"), []byte("project 2 chapter"), 0644); err != nil {
		t.Fatalf("seed p2 chapter: %v", err)
	}

	files, err := s.listChapterFiles()
	if err != nil {
		t.Fatalf("list default chapters: %v", err)
	}
	if len(files) != 1 || files[0].Name != "第01章.md" {
		t.Fatalf("expected only default chapter, got %#v", files)
	}

	if err := s.switchProject("p2"); err != nil {
		t.Fatalf("switch project: %v", err)
	}

	files, err = s.listChapterFiles()
	if err != nil {
		t.Fatalf("list p2 chapters: %v", err)
	}
	if len(files) != 1 || files[0].Name != "第02章.md" {
		t.Fatalf("expected only p2 chapter, got %#v", files)
	}
}
