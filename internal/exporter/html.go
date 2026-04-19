package exporter

import (
	"fmt"
	"html"
	"strings"
)

// ManuscriptToHTML converts a manuscript markdown export into a self-contained HTML5 document.
func ManuscriptToHTML(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")

	var body strings.Builder
	var paraLines []string

	flushPara := func() {
		if len(paraLines) == 0 {
			return
		}
		text := html.EscapeString(strings.Join(paraLines, "\n"))
		body.WriteString("<p>" + strings.ReplaceAll(text, "\n", "<br>") + "</p>\n")
		paraLines = paraLines[:0]
	}

	inMetadata := false
	var metaLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "<!-- manuscript-metadata") {
			flushPara()
			inMetadata = true
			metaLines = metaLines[:0]
			continue
		}
		if inMetadata {
			if strings.TrimSpace(line) == "-->" {
				inMetadata = false
				if len(metaLines) > 0 {
					body.WriteString("<details class=\"manuscript-meta\"><summary>場景規劃</summary><div>\n")
					for _, ml := range metaLines {
						body.WriteString(html.EscapeString(ml) + "<br>\n")
					}
					body.WriteString("</div></details>\n")
				}
			} else {
				metaLines = append(metaLines, line)
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "# "):
			flushPara()
			body.WriteString(fmt.Sprintf("<h1>%s</h1>\n", html.EscapeString(line[2:])))
		case strings.HasPrefix(line, "### "):
			flushPara()
			body.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(line[4:])))
		case strings.HasPrefix(line, "## "):
			flushPara()
			body.WriteString(fmt.Sprintf("<h2>%s</h2>\n", html.EscapeString(line[3:])))
		case strings.TrimSpace(line) == "":
			flushPara()
		default:
			paraLines = append(paraLines, line)
		}
	}
	flushPara()

	return manuscriptHTMLTemplate(body.String())
}

func manuscriptHTMLTemplate(body string) string {
	return `<!DOCTYPE html>
<html lang="zh-TW">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>手稿匯出</title>
<style>
  body { font-family: "Noto Serif TC", serif; max-width: 800px; margin: 0 auto; padding: 2rem; line-height: 1.9; color: #1a1a1a; }
  h1 { font-size: 2rem; border-bottom: 2px solid #333; padding-bottom: .5rem; margin-top: 2rem; }
  h2 { font-size: 1.5rem; margin-top: 2rem; color: #222; }
  h3 { font-size: 1.1rem; margin-top: 1.5rem; color: #555; }
  p  { margin: 1rem 0; text-indent: 2em; }
  details.manuscript-meta { margin: 1rem 0; background: #f5f5f5; border: 1px solid #ddd; border-radius: 4px; padding: .5rem 1rem; font-size: .85rem; color: #555; }
  details.manuscript-meta summary { cursor: pointer; font-weight: bold; }
  @media print { details.manuscript-meta { display: none; } }
</style>
</head>
<body>
` + body + `</body>
</html>`
}
