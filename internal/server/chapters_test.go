package server

import (
	"novel-assistant/internal/config"
	"novel-assistant/internal/vectorstore"
	"testing"
)

// ─── scene parser tests ───────────────────────────────────────────────────────

func TestParseScenesReturnsNilForPlainChapter(t *testing.T) {
	t.Parallel()

	content := "Lin Hao pushed open the door.\nHe looked around slowly."
	scenes := parseScenes(content)
	if scenes != nil {
		t.Fatalf("expected nil for chapter without markers, got %v", scenes)
	}
}

func TestParseScenesWithNumberedMarkers(t *testing.T) {
	t.Parallel()

	content := `## Scene 1
First scene content.

## Scene 2
Second scene content.`

	scenes := parseScenes(content)
	if len(scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(scenes))
	}
	if scenes[0].Index != 1 {
		t.Fatalf("expected index 1, got %d", scenes[0].Index)
	}
	if scenes[0].Title != "Scene 1" {
		t.Fatalf("expected title \"Scene 1\", got %q", scenes[0].Title)
	}
	if scenes[0].Content != "First scene content." {
		t.Fatalf("unexpected scene 1 content: %q", scenes[0].Content)
	}
	if scenes[1].Title != "Scene 2" {
		t.Fatalf("expected title \"Scene 2\", got %q", scenes[1].Title)
	}
}

func TestParseScenesWithTitledMarkers(t *testing.T) {
	t.Parallel()

	content := `## Scene 1: The Confrontation
Lin Hao stood his ground.

## Scene 2: The Rain
Zhang Lei waited in the rain.`

	scenes := parseScenes(content)
	if len(scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(scenes))
	}
	if scenes[0].Title != "Scene 1: The Confrontation" {
		t.Fatalf("unexpected title: %q", scenes[0].Title)
	}
	if scenes[1].Title != "Scene 2: The Rain" {
		t.Fatalf("unexpected title: %q", scenes[1].Title)
	}
}

func TestParseScenesDoesNotMatchMidLineHeaders(t *testing.T) {
	t.Parallel()

	// "## Scene 1" preceded by spaces on the same logical line should not match
	// because the regex requires start of line (^).
	content := "Some text\n  ## Scene 1\nMore text"
	scenes := parseScenes(content)
	if scenes != nil {
		t.Fatalf("expected nil for non-leading scene header, got %v", scenes)
	}
}

func TestSceneByTitle(t *testing.T) {
	t.Parallel()

	scenes := []Scene{
		{Index: 1, Title: "Scene 1: Opening", Content: "..."},
		{Index: 2, Title: "Scene 2: Rain", Content: "..."},
	}

	got := sceneByTitle(scenes, "Scene 2: Rain")
	if got == nil {
		t.Fatal("expected to find scene, got nil")
	}
	if got.Index != 2 {
		t.Fatalf("expected index 2, got %d", got.Index)
	}

	missing := sceneByTitle(scenes, "Scene 99")
	if missing != nil {
		t.Fatalf("expected nil for missing scene, got %v", missing)
	}
}

func TestNormalizeChapterName(t *testing.T) {
	t.Parallel()

	name, err := normalizeChapterName("第03章_雨夜對峙")
	if err != nil {
		t.Fatalf("expected valid chapter name, got error: %v", err)
	}
	if name != "第03章_雨夜對峙.md" {
		t.Fatalf("unexpected normalized name: %s", name)
	}
}

func TestNormalizeChapterNameRejectsTraversal(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"../secret.md", "folder/chapter.md", "folder\\chapter.md", ""} {
		if _, err := normalizeChapterName(raw); err == nil {
			t.Fatalf("expected invalid chapter name for %q", raw)
		}
	}
}

func TestSaveAndLoadChapterFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := &Server{
		cfg: &config.Config{DataDir: dir},
	}

	saved, err := s.saveChapterFile("第01章_開場", "林昊推門而入。")
	if err != nil {
		t.Fatalf("expected save to succeed, got error: %v", err)
	}
	if saved.Name != "第01章_開場.md" {
		t.Fatalf("unexpected saved filename: %s", saved.Name)
	}

	loaded, err := s.loadChapterFile(saved.Name)
	if err != nil {
		t.Fatalf("expected load to succeed, got error: %v", err)
	}
	if loaded.Content != "林昊推門而入。" {
		t.Fatalf("unexpected loaded content: %s", loaded.Content)
	}
}

func TestChunkChapterUsesSceneMarkers(t *testing.T) {
	t.Parallel()

	content := "## Scene 1: Opening\nLin Hao stepped in.\n\n## Scene 2: Rain\nZhang Lei waited."
	chunks := chunkChapter("第03章.md", content)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	assertChunk(t, chunks[0], vectorstore.Document{
		ID:           "chapter_第03章.md_scene_1",
		Type:         "chapter",
		Content:      "Lin Hao stepped in.",
		ChapterFile:  "第03章.md",
		ChapterIndex: 3,
		SceneIndex:   1,
		ChunkType:    "scene",
	})
	assertChunk(t, chunks[1], vectorstore.Document{
		ID:           "chapter_第03章.md_scene_2",
		Type:         "chapter",
		Content:      "Zhang Lei waited.",
		ChapterFile:  "第03章.md",
		ChapterIndex: 3,
		SceneIndex:   2,
		ChunkType:    "scene",
	})
}

func TestChunkChapterFallsBackToParagraphs(t *testing.T) {
	t.Parallel()

	content := "第一段。\n\n第二段。\n\n\n\n第三段。"
	chunks := chunkChapter("第08章.md", content)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 paragraph chunks, got %d", len(chunks))
	}
	assertChunk(t, chunks[0], vectorstore.Document{
		ID:           "chapter_第08章.md_para_1",
		Type:         "chapter",
		Content:      "第一段。",
		ChapterFile:  "第08章.md",
		ChapterIndex: 8,
		SceneIndex:   1,
		ChunkType:    "paragraph",
	})
	assertChunk(t, chunks[2], vectorstore.Document{
		ID:           "chapter_第08章.md_para_3",
		Type:         "chapter",
		Content:      "第三段。",
		ChapterFile:  "第08章.md",
		ChapterIndex: 8,
		SceneIndex:   3,
		ChunkType:    "paragraph",
	})
}

func TestChunkChapterReturnsEmptySliceForBlankContent(t *testing.T) {
	t.Parallel()

	chunks := chunkChapter("第01章.md", " \n\t ")
	if len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %#v", chunks)
	}
}

func TestExtractChapterIndexFromFilename(t *testing.T) {
	t.Parallel()

	if got := extractChapterIndex("第03章.md"); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := extractChapterIndex("prologue.md"); got != 0 {
		t.Fatalf("expected fallback 0, got %d", got)
	}
	if got := extractChapterIndex("番外.md"); got != 0 {
		t.Fatalf("expected fallback 0, got %d", got)
	}
}

func assertChunk(t *testing.T, got, want vectorstore.Document) {
	t.Helper()
	if got.ID != want.ID || got.Type != want.Type || got.Content != want.Content || got.ChapterFile != want.ChapterFile || got.ChapterIndex != want.ChapterIndex || got.SceneIndex != want.SceneIndex || got.ChunkType != want.ChunkType {
		t.Fatalf("unexpected chunk: got %#v want %#v", got, want)
	}
}
