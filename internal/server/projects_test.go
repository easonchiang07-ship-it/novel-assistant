package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
	"novel-assistant/internal/workspace"

	"github.com/gin-gonic/gin"
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

func TestSwitchProject_KeepsGlobalModelSettings(t *testing.T) {
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

	if err := workspace.SaveIndex(workspace.Index{
		Active: "default",
		Names:  []string{"default", "p2"},
	}); err != nil {
		t.Fatalf("save workspace index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("web", "templates"), 0755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join("web", "templates", "dummy.html"), []byte(`{{define "dummy.html"}}ok{{end}}`), 0644); err != nil {
		t.Fatalf("seed dummy template: %v", err)
	}

	globalStore := projectsettings.New(filepath.Join("data", "project_settings.json"), projectsettings.Settings{
		OllamaURL:  "http://global-ollama:11434",
		LLMModel:   "global-llm",
		EmbedModel: "global-embed",
		Port:       "18080",
		DataDir:    "data",
	})
	if err := globalStore.Save(); err != nil {
		t.Fatalf("save global settings: %v", err)
	}

	customStore := projectsettings.New(filepath.Join("workspaces", "p2", "project_settings.json"), projectsettings.Settings{
		OllamaURL:  "http://project-ollama:11434",
		LLMModel:   "project-llm",
		EmbedModel: "project-embed",
		Port:       "28080",
		DataDir:    filepath.Join("workspaces", "p2"),
	})
	if err := customStore.Save(); err != nil {
		t.Fatalf("save custom settings: %v", err)
	}

	s, err := New(&config.Config{
		DataDir:    "data",
		OllamaURL:  "http://bootstrap-ollama:11434",
		LLMModel:   "bootstrap-llm",
		EmbedModel: "bootstrap-embed",
		Port:       "8080",
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	if err := s.switchProject("p2"); err != nil {
		t.Fatalf("switch project: %v", err)
	}

	if s.cfg.OllamaURL != "http://global-ollama:11434" {
		t.Fatalf("expected global ollama url to stay global, got %q", s.cfg.OllamaURL)
	}
	if s.cfg.LLMModel != "global-llm" {
		t.Fatalf("expected global llm model to stay global, got %q", s.cfg.LLMModel)
	}
	if s.cfg.EmbedModel != "global-embed" {
		t.Fatalf("expected global embed model to stay global, got %q", s.cfg.EmbedModel)
	}
	if s.cfg.Port != "18080" {
		t.Fatalf("expected global port to stay global, got %q", s.cfg.Port)
	}
}

func TestHandleSwitchProject_DoesNotChangeStateWhenIndexSaveFails(t *testing.T) {
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

	if err := workspace.SaveIndex(workspace.Index{
		Active: "default",
		Names:  []string{"default", "p2"},
	}); err != nil {
		t.Fatalf("save workspace index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("web", "templates"), 0755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join("web", "templates", "dummy.html"), []byte(`{{define "dummy.html"}}ok{{end}}`), 0644); err != nil {
		t.Fatalf("seed dummy template: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("workspaces", "p2"), 0755); err != nil {
		t.Fatalf("mkdir p2: %v", err)
	}

	s, err := New(&config.Config{
		DataDir:    "data",
		OllamaURL:  "http://localhost:11434",
		LLMModel:   "llama3.2",
		EmbedModel: "nomic-embed-text",
		Port:       "8080",
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	originalDataDir := s.currentState().dataDir

	prevSave := saveWorkspaceIndex
	saveWorkspaceIndex = func(idx workspace.Index) error {
		return errors.New("disk full")
	}
	defer func() {
		saveWorkspaceIndex = prevSave
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "name", Value: "p2"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/projects/p2/switch", http.NoBody)

	s.handleSwitchProject(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when index save fails, got %d", w.Code)
	}
	if s.currentState().dataDir != originalDataDir {
		t.Fatalf("expected active state to stay %q, got %q", originalDataDir, s.currentState().dataDir)
	}

	idx, err := workspace.EnsureIndex()
	if err != nil {
		t.Fatalf("reload workspace index: %v", err)
	}
	if idx.Active != "default" {
		t.Fatalf("expected persisted active project to stay default, got %q", idx.Active)
	}
}
