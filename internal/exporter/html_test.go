package exporter_test

import (
	"strings"
	"testing"

	"novel-assistant/internal/exporter"
)

func TestManuscriptToHTMLConvertsHeadings(t *testing.T) {
	md := "# 手稿\n\n## 第一章\n\n### 場景一\n\n正文內容。\n"
	out := exporter.ManuscriptToHTML(md)

	for _, want := range []string{"<h1>手稿</h1>", "<h2>第一章</h2>", "<h3>場景一</h3>", "<p>正文內容。</p>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestManuscriptToHTMLRendersMetadataAsDetails(t *testing.T) {
	md := "## 第一章\n\n<!-- manuscript-metadata\n### Scene 1\n- 摘要：開頭\n-->\n"
	out := exporter.ManuscriptToHTML(md)

	if !strings.Contains(out, "<details class=\"manuscript-meta\">") {
		t.Fatalf("expected metadata details block, got %q", out)
	}
	if !strings.Contains(out, "摘要：開頭") {
		t.Fatalf("expected metadata content in output, got %q", out)
	}
}

func TestManuscriptToHTMLEscapesHTML(t *testing.T) {
	md := "## <script>alert(1)</script>\n"
	out := exporter.ManuscriptToHTML(md)

	if strings.Contains(out, "<script>") {
		t.Fatalf("expected HTML to be escaped in output, got %q", out)
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected escaped heading content, got %q", out)
	}
}

func TestManuscriptToHTMLIsValidHTML5(t *testing.T) {
	out := exporter.ManuscriptToHTML("# Title\n\nBody.\n")

	if !strings.Contains(out, "<!DOCTYPE html>") || !strings.Contains(out, "</html>") {
		t.Fatalf("expected valid HTML5 document wrapper, got %q", out)
	}
}
