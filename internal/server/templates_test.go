package server

import (
	"os"
	"strings"
	"testing"
)

func TestProjectTemplateFilesReturnsUrbanFantasyStarterFiles(t *testing.T) {
	t.Parallel()

	files, err := projectTemplateFiles("urban-fantasy")
	if err != nil {
		t.Fatalf("expected template files, got error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected non-empty template files")
	}
}

func TestProjectTemplateFilesRejectsUnknownTemplate(t *testing.T) {
	t.Parallel()

	if _, err := projectTemplateFiles("unknown"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestStylesTemplateRenderStyleAnalysisAvoidsInnerHTMLInterpolation(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../web/templates/styles.html")
	if err != nil {
		t.Fatalf("read styles template: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "摘要：${analysis.summary") {
		t.Fatal("expected style analysis rendering to avoid interpolating untrusted values into innerHTML")
	}
	if !strings.Contains(text, "summary.textContent = '摘要：'") {
		t.Fatal("expected style analysis rendering to use textContent")
	}
}

func TestCheckTemplateRendersSourceLocationLabel(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../web/templates/check.html")
	if err != nil {
		t.Fatalf("read check template: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "function formatSourceLocation(item)") {
		t.Fatal("expected check template to define source location formatter")
	}
	if !strings.Contains(text, "第 ' + item.chapter_index + ' 章") {
		t.Fatal("expected source formatter to mention chapter index labels")
	}
	if !strings.Contains(text, "Scene ' + item.scene_index") {
		t.Fatal("expected source formatter to mention scene labels")
	}
	if !strings.Contains(text, "段落 ' + item.scene_index") {
		t.Fatal("expected source formatter to mention paragraph labels")
	}
}
