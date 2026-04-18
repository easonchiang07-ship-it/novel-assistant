package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCharacterParsesSupportedFields(t *testing.T) {
	t.Parallel()

	char := parseCharacter(`# 角色：林昊
- 個性：沉默寡言
- 核心恐懼：失去摯友
- 行為模式：先觀察再行動
- 弱點：對家人毫無防備
- 成長限制：不主動求助
- 說話風格：話少`)

	if char.Name != "林昊" {
		t.Fatalf("expected name 林昊, got %q", char.Name)
	}
	if char.Personality != "沉默寡言" {
		t.Fatalf("expected personality parsed, got %q", char.Personality)
	}
	if char.SpeechStyle != "話少" {
		t.Fatalf("expected speech style parsed, got %q", char.SpeechStyle)
	}
}

func TestParseStyleGuideParsesSupportedFields(t *testing.T) {
	t.Parallel()

	style := parseStyleGuide(`# 風格：主線敘事
- 敘事視角：第三人稱有限視角
- 句式風格：短句，少修飾
- 節奏感：穩定推進
- 語氣：克制冷靜
- 禁忌：避免全知旁白`)

	if style.Name != "主線敘事" {
		t.Fatalf("expected style name parsed, got %q", style.Name)
	}
	if style.Perspective != "第三人稱有限視角" {
		t.Fatalf("expected perspective parsed, got %q", style.Perspective)
	}
	if style.Forbidden != "避免全知旁白" {
		t.Fatalf("expected forbidden parsed, got %q", style.Forbidden)
	}
}

func TestLoadIndexesCharacterAppearancesFromChapters(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "characters"), 0755); err != nil {
		t.Fatalf("mkdir characters: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "chapters"), 0755); err != nil {
		t.Fatalf("mkdir chapters: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "characters", "林昊.md"), []byte("# 角色：林昊\n- 個性：沉默"), 0644); err != nil {
		t.Fatalf("write character: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "第01章_開場.md"), []byte("林昊走進房間。"), 0644); err != nil {
		t.Fatalf("write chapter 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "第02章_雨夜.md"), []byte("張雷在雨中等待。"), 0644); err != nil {
		t.Fatalf("write chapter 2: %v", err)
	}

	manager := NewManager(dir)
	if err := manager.Load(); err != nil {
		t.Fatalf("load manager: %v", err)
	}

	char := manager.FindByName("林昊")
	if char == nil {
		t.Fatal("expected character 林昊 to be loaded")
	}
	if len(char.Appearances) != 1 {
		t.Fatalf("expected 1 appearance, got %d", len(char.Appearances))
	}
	if char.Appearances[0].ChapterTitle != "第01章_開場" {
		t.Fatalf("unexpected appearance title: %s", char.Appearances[0].ChapterTitle)
	}
}
