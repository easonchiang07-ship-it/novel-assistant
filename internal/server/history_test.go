package server

import (
	"strings"
	"testing"
	"time"

	"novel-assistant/internal/reviewhistory"
)

func TestBuildHistoryGroupsKeepsChapterBuckets(t *testing.T) {
	t.Parallel()

	groups := buildHistoryGroups([]*reviewhistory.Entry{
		{ChapterTitle: "第一章"},
		{ChapterTitle: "第二章"},
		{ChapterTitle: "第一章"},
	})

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].ChapterTitle != "第一章" || len(groups[0].Entries) != 2 {
		t.Fatalf("unexpected first group: %#v", groups[0])
	}
}

func TestFormatHistoryMarkdownIncludesKeyMetadata(t *testing.T) {
	t.Parallel()

	output := string(formatHistoryMarkdown([]*reviewhistory.Entry{
		{
			ChapterTitle:   "第一章",
			Kind:           "rewrite",
			ChapterVersion: 3,
			KindVersion:    2,
			RewriteMode:    "style",
			ChapterFile:    "第01章.md",
			Checks:         []string{"behavior"},
			Styles:         []string{"主線敘事"},
			Sources:        []string{"world:城市規則"},
			Result:         "修稿內容",
			CreatedAt:      time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		},
	}))

	for _, fragment := range []string{"# 審查歷史匯出", "## 第一章", "- 章節版本序號：第 3 筆", "- 類型版本序號：第 2 次修稿", "- 修稿模式：style", "- 章節檔案：第01章.md", "修稿內容"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHistoryEditorContentRemovesCompletionMarker(t *testing.T) {
	t.Parallel()

	content := historyEditorContent(&reviewhistory.Entry{
		Result: "修稿後內容\n\n---\n修稿完成\n",
	})
	if content != "修稿後內容" {
		t.Fatalf("unexpected editor content: %q", content)
	}
}
