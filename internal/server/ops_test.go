package server

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel-assistant/internal/config"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/tracker"
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
