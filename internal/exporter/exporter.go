package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ExportMarkdown(title, chapter, content, outputDir string) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("**章節：** %s\n\n", chapter))
	sb.WriteString(fmt.Sprintf("**審查時間：** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")
	sb.WriteString(content)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("report_%s.md", time.Now().Format("20060102_150405"))
	path := filepath.Join(outputDir, filename)
	return path, os.WriteFile(path, []byte(sb.String()), 0644)
}
