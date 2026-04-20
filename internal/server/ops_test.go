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
	"time"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"

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

	s := &Server{
		cfg:     &config.Config{DataDir: dir},
		project: projectsettings.New(filepath.Join(dir, "project_settings.json"), projectsettings.Settings{DataDir: dir}),
	}
	item, err := s.createBackupSnapshot()
	if err != nil {
		t.Fatalf("expected backup snapshot, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", item.Name, "chapters", "第01章.md")); err != nil {
		t.Fatalf("expected chapter in backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", item.Name, ".backup_manifest.json")); err != nil {
		t.Fatalf("expected backup manifest: %v", err)
	}
}

func TestCreateBackupSnapshotPrunesOldSnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for i, name := range []string{"backup_newest", "backup_middle", "backup_oldest"} {
		target := filepath.Join(backupDir, name)
		if err := os.MkdirAll(target, 0755); err != nil {
			t.Fatal(err)
		}
		ts := now.Add(-time.Duration(i) * time.Hour)
		if err := writeBackupPreview(target, backupPreview{Name: name, CreatedAt: ts}); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(target, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := pruneOldBackups(backupDir, 2, nil)
	if err != nil {
		t.Fatalf("prune backups: %v", err)
	}
	if len(removed) != 1 || removed[0] != "backup_oldest" {
		t.Fatalf("expected oldest backup pruned, got %v", removed)
	}
}

func TestHandleRestoreBackupCreatesSafetySnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)
	if _, err := s.saveChapterFile("第01章", "目前內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	item, err := s.createBackupSnapshot()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "第01章.md"), []byte("被覆蓋前的現況"), 0644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/backups/restore", strings.NewReader(`{"name":"`+item.Name+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	s.handleRestoreBackup(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected restore success, got %d %s", w.Code, w.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, "chapters", "第01章.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "目前內容" {
		t.Fatalf("expected restored chapter content, got %q", content)
	}

	items, err := listBackupItems(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	foundSafety := false
	for _, candidate := range items {
		if strings.HasPrefix(candidate.Name, "pre_restore_") {
			foundSafety = true
			break
		}
	}
	if !foundSafety {
		t.Fatalf("expected safety snapshot before restore, got %#v", items)
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

func TestBuildManuscriptMarkdownAppendsReviewsAppendix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}
	s.history.Add(&reviewhistory.Entry{Kind: "review", ChapterFile: "第01章.md", ChapterTitle: "第01章"})
	s.history.Add(&reviewhistory.Entry{Kind: "rewrite", ChapterFile: "第01章.md", ChapterTitle: "第01章", RewriteMode: "強化張力"})

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Appendix: manuscriptAppendixOptions{Reviews: true},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}

	for _, fragment := range []string{
		"# 附錄",
		"## 審查與修稿歷史",
		"### 第01章",
		"第 1 次審查",
		"第 1 次修稿",
		"模式：強化張力",
	} {
		if !strings.Contains(manuscript, fragment) {
			t.Fatalf("expected reviews appendix fragment %q, got %q", fragment, manuscript)
		}
	}
}

func TestBuildManuscriptMarkdownAppendsTrackerAppendix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if _, err := s.saveChapterFile("第02章", "內容"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	s.timeline.Add(&tracker.TimelineEvent{Chapter: 1, Scene: "夜港塔", Description: "主角抵達現場"})
	s.foreshadow.Add(&tracker.Foreshadowing{Chapter: 1, Description: "塔上的異象", PlantedIn: "第01章"})
	s.relationships.Upsert(&tracker.Relationship{From: "林昊", To: "張雷", Status: "信任", Note: "一起追查夜港塔"})

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Appendix: manuscriptAppendixOptions{Tracker: true},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}

	for _, fragment := range []string{
		"# 附錄",
		"## 追蹤資料",
		"### 時間軸",
		"第01章：夜港塔：主角抵達現場",
		"### 伏筆",
		"第01章：塔上的異象（未回收）",
		"### 角色關係",
		"林昊 ↔ 張雷：信任",
		"一起追查夜港塔",
	} {
		if !strings.Contains(manuscript, fragment) {
			t.Fatalf("expected tracker appendix fragment %q, got %q", fragment, manuscript)
		}
	}
}

func TestBuildManuscriptMarkdownMatchesNonStandardChapterNamesInTrackerAppendix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("prologue", "序章內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}
	s.timeline.Add(&tracker.TimelineEvent{Chapter: 0, Scene: "開場", Description: "序章事件"})
	s.foreshadow.Add(&tracker.Foreshadowing{Chapter: 0, Description: "序章伏筆", PlantedIn: "prologue"})

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{{Name: "prologue.md"}},
		Appendix:   manuscriptAppendixOptions{Tracker: true},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}
	for _, fragment := range []string{
		"## 追蹤資料",
		"prologue：開場：序章事件",
		"prologue：序章伏筆（未回收）",
	} {
		if !strings.Contains(manuscript, fragment) {
			t.Fatalf("expected nonstandard chapter appendix fragment %q, got %q", fragment, manuscript)
		}
	}
}

func TestBuildManuscriptMarkdownFiltersAppendixToSelectedChapters(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if _, err := s.saveChapterFile("第02章", "內容"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	s.history.Add(&reviewhistory.Entry{Kind: "review", ChapterFile: "第01章.md", ChapterTitle: "第01章"})
	s.history.Add(&reviewhistory.Entry{Kind: "review", ChapterFile: "第02章.md", ChapterTitle: "第02章"})
	s.timeline.Add(&tracker.TimelineEvent{Chapter: 1, Scene: "A", Description: "第一章事件"})
	s.timeline.Add(&tracker.TimelineEvent{Chapter: 2, Scene: "B", Description: "第二章事件"})
	s.foreshadow.Add(&tracker.Foreshadowing{Chapter: 1, Description: "第一章伏筆", PlantedIn: "第01章"})
	s.foreshadow.Add(&tracker.Foreshadowing{Chapter: 2, Description: "第二章伏筆", PlantedIn: "第02章"})

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{{Name: "第01章.md"}},
		Appendix: manuscriptAppendixOptions{
			Reviews: true,
			Tracker: true,
		},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}

	for _, fragment := range []string{"第01章", "第一章事件", "第一章伏筆"} {
		if !strings.Contains(manuscript, fragment) {
			t.Fatalf("expected selected chapter appendix data %q, got %q", fragment, manuscript)
		}
	}
	for _, fragment := range []string{"第02章", "第二章事件", "第二章伏筆"} {
		if strings.Contains(manuscript, fragment) {
			t.Fatalf("expected appendix to exclude unselected chapter data %q, got %q", fragment, manuscript)
		}
	}
}

func TestBuildManuscriptMarkdownKeepsRelationshipsUnfilteredInAppendix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", "只出現林昊"); err != nil {
		t.Fatalf("save chapter 1: %v", err)
	}
	if _, err := s.saveChapterFile("第02章", "只出現張雷"); err != nil {
		t.Fatalf("save chapter 2: %v", err)
	}
	s.relationships.Upsert(&tracker.Relationship{From: "林昊", To: "張雷", Status: "信任"})

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Selections: []manuscriptExportSelection{{Name: "第01章.md"}},
		Appendix:   manuscriptAppendixOptions{Tracker: true},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}
	if !strings.Contains(manuscript, "林昊 ↔ 張雷：信任") {
		t.Fatalf("expected relationships appendix to remain unfiltered, got %q", manuscript)
	}
}

func TestBuildManuscriptMarkdownSkipsAppendixHeadingWhenNoAppendixData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newOpsTestServer(dir)

	if _, err := s.saveChapterFile("第01章", "內容"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	manuscript, err := s.buildManuscriptMarkdown(manuscriptExportRequest{
		Appendix: manuscriptAppendixOptions{
			Reviews: true,
			Tracker: true,
		},
	})
	if err != nil {
		t.Fatalf("build manuscript markdown: %v", err)
	}
	if strings.Contains(manuscript, "# 附錄") {
		t.Fatalf("expected empty appendix to be omitted, got %q", manuscript)
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
	project := projectsettings.New(filepath.Join(dir, "project_settings.json"), projectsettings.Settings{DataDir: dir})
	return &Server{
		cfg:           &config.Config{DataDir: dir},
		project:       project,
		profiles:      profile.NewManager(dir),
		store:         vectorstore.New(filepath.Join(dir, "store.json")),
		rules:         reviewrules.New(filepath.Join(dir, "review_rules.json")),
		history:       reviewhistory.New(filepath.Join(dir, "reviews.json")),
		timeline:      tracker.NewTimelineTracker(filepath.Join(dir, "timeline.json")),
		foreshadow:    tracker.NewForeshadowTracker(filepath.Join(dir, "foreshadow.json")),
		relationships: tracker.NewRelationshipTracker(filepath.Join(dir, "relationships.json")),
	}
}
