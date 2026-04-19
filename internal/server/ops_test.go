package server

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"

	"github.com/gin-gonic/gin"
)

func TestBuildChapterBundleMarkdownIncludesLinkedData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "chapters"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "第01章.md"), []byte("林昊走進夜港塔。"), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg:           &config.Config{DataDir: dir},
		history:       reviewhistory.New(filepath.Join(dir, "reviews.json")),
		timeline:      tracker.NewTimelineTracker(filepath.Join(dir, "timeline.json")),
		foreshadow:    tracker.NewForeshadowTracker(filepath.Join(dir, "foreshadow.json")),
		relationships: tracker.NewRelationshipTracker(filepath.Join(dir, "relationships.json")),
		profiles:      &profile.Manager{Characters: []*profile.Character{{Name: "林昊"}}},
	}

	s.history.Add(&reviewhistory.Entry{Kind: "review", ChapterFile: "第01章.md", ChapterTitle: "第01章"})
	s.timeline.Add(&tracker.TimelineEvent{Scene: "夜港塔", Description: "主角抵達現場", Chapter: 1})
	s.foreshadow.Add(&tracker.Foreshadowing{Description: "塔上的異象", Chapter: 1})
	s.relationships.Upsert(&tracker.Relationship{From: "林昊", To: "張雷", Status: "信任"})

	report, err := s.buildChapterBundleMarkdown("第01章.md")
	if err != nil {
		t.Fatalf("expected bundle markdown, got error: %v", err)
	}
	for _, fragment := range []string{"# 章節完整報告", "夜港塔", "塔上的異象", "林昊 ↔ 張雷"} {
		if !strings.Contains(report, fragment) {
			t.Fatalf("expected report to contain %q, got %q", fragment, report)
		}
	}
}

func TestCreateBackupSnapshotCopiesMarkdownAndJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "chapters"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "第01章.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "timeline.json"), []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: &config.Config{DataDir: dir}}
	item, err := s.createBackupSnapshot()
	if err != nil {
		t.Fatalf("expected backup snapshot, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", item.Name, "chapters", "第01章.md")); err != nil {
		t.Fatalf("expected chapter in backup: %v", err)
	}
}

func TestZipHelperWritesReadableFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := addZipFile(zw, path, "sample.md"); err != nil {
		t.Fatalf("expected zip add success, got %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hello" {
		t.Fatalf("unexpected zip contents: %q", data)
	}
}

func TestBuildManuscriptMarkdownUsesSavedChapterAndSceneOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第02章", "第二章內容"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	if _, err := s.saveChapterFile("第01章", `前言

## Scene 1: Opening
Open.

## Scene 2: Rain
Rain.`); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if err := s.saveChapterOrder([]string{"第02章.md", "第01章.md"}); err != nil {
		t.Fatalf("save chapter order: %v", err)
	}
	if err := s.saveScenePlanOrder("第01章.md", []string{"Scene 2: Rain", "Scene 1: Opening"}); err != nil {
		t.Fatalf("save scene order: %v", err)
	}

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}

	chapter2Pos := strings.Index(manuscript, "## 第02章")
	chapter1Pos := strings.Index(manuscript, "## 第01章")
	if chapter2Pos == -1 || chapter1Pos == -1 || chapter2Pos > chapter1Pos {
		t.Fatalf("expected chapter order to follow saved order, got %q", manuscript)
	}
	rainPos := strings.Index(manuscript, "## Scene 2: Rain")
	openPos := strings.Index(manuscript, "## Scene 1: Opening")
	if rainPos == -1 || openPos == -1 || rainPos > openPos {
		t.Fatalf("expected scene order to follow saved order, got %q", manuscript)
	}
	if strings.Contains(manuscript, "第二章內容\n\n\n\n## 第01章") {
		t.Fatalf("expected manuscript chapters to be separated by two newlines, got %q", manuscript)
	}
}

func TestBuildManuscriptMarkdownFiltersChaptersAndScenes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第02章", "第二章內容"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	if _, err := s.saveChapterFile("第01章", `前言

## Scene 1: Opening
Open.

## Scene 2: Rain
Rain.`); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{
			{Name: "第01章.md", Scenes: []string{"Scene 2: Rain"}},
		},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}
	if strings.Contains(manuscript, "## 第02章") {
		t.Fatalf("expected chapter filter to exclude 第02章, got %q", manuscript)
	}
	if strings.Contains(manuscript, "## Scene 1: Opening") {
		t.Fatalf("expected scene filter to exclude Scene 1, got %q", manuscript)
	}
	if !strings.Contains(manuscript, "## Scene 2: Rain") {
		t.Fatalf("expected selected scene to remain, got %q", manuscript)
	}
	if !strings.Contains(manuscript, "前言") {
		t.Fatalf("expected chapter preamble to be preserved, got %q", manuscript)
	}
}

func TestBuildManuscriptMarkdownIncludesMetadataCommentWhenRequested(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", `## Scene 1: Opening
Open.`); err != nil {
		t.Fatalf("save chapter: %v", err)
	}
	if err := s.saveScenePlan("第01章.md", scenePlan{
		Title:    "Scene 1: Opening",
		Synopsis: "主角登場",
		POV:      "林昊",
		Conflict: "是否進門",
		Purpose:  "建立懸念",
	}); err != nil {
		t.Fatalf("save scene plan: %v", err)
	}

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		IncludeMetadata: true,
		Selections: []manuscriptExportSelection{
			{Name: "第01章.md", Scenes: []string{"Scene 1: Opening"}},
		},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}
	for _, fragment := range []string{
		"<!-- manuscript-metadata",
		"### Scene 1: Opening",
		"- 摘要：主角登場",
		"- POV：林昊",
		"- 衝突：是否進門",
		"- 目的：建立懸念",
	} {
		if !strings.Contains(manuscript, fragment) {
			t.Fatalf("expected metadata fragment %q, got %q", fragment, manuscript)
		}
	}
}

func TestHandleExportManuscriptAllowsEmptyBody(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)
	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/manuscript/export", http.NoBody)

	s.handleExportManuscript(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected empty body export to pass, got %d %s", w.Code, w.Body.String())
	}
}

func TestBuildManuscriptMarkdownRejectsUnknownSelectionOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)
	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	_, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{{Name: "不存在.md"}},
	})
	if err == nil || !strings.Contains(err.Error(), "沒有符合條件") {
		t.Fatalf("expected filtered export error, got %v", err)
	}
}

func TestBuildManuscriptMarkdownRejectsUnknownSceneSelection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)
	if _, err := s.saveChapterFile("第01章", `前言

## Scene 1: Opening
Open.`); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	_, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{
			{Name: "第01章.md", Scenes: []string{"Scene 9: Missing"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "找不到指定場景") {
		t.Fatalf("expected unknown scene selection error, got %v", err)
	}
}

func newOpsTestServer(dir string) *Server {
	return &Server{
		cfg:        &config.Config{DataDir: dir},
		profiles:   profile.NewManager(dir),
		rules:      reviewrules.New(filepath.Join(dir, "review_rules.json")),
		history:    reviewhistory.New(filepath.Join(dir, "reviews.json")),
		timeline:   tracker.NewTimelineTracker(filepath.Join(dir, "timeline.json")),
		foreshadow: tracker.NewForeshadowTracker(filepath.Join(dir, "foreshadow.json")),
	}
}
