package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"novel-assistant/internal/exporter"
	"novel-assistant/internal/extractor"

	"github.com/gin-gonic/gin"
)

type backupRequest struct {
	Name string `json:"name"`
}

type chapterReportRequest struct {
	Name string `json:"name"`
}

type manuscriptExportSelection struct {
	Name   string   `json:"name"`
	Scenes []string `json:"scenes,omitempty"`
}

type manuscriptExportRequest struct {
	Selections      []manuscriptExportSelection `json:"selections"`
	IncludeMetadata bool                        `json:"include_metadata"`
	Format          string                      `json:"format"`
}

type backupItem struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) backupDir() string {
	return filepath.Join(s.cfg.DataDir, "backups")
}

func listBackupItems(dir string) ([]backupItem, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]backupItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, backupItem{Name: entry.Name(), CreatedAt: info.ModTime()})
	}
	return items, nil
}

func snapshotCopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func copySnapshotTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "backups" || strings.HasPrefix(rel, "backups"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if filepath.Ext(d.Name()) != ".md" && filepath.Ext(d.Name()) != ".json" && d.Name() != ".gitkeep" {
			return nil
		}
		return snapshotCopyFile(path, target)
	})
}

func (s *Server) createBackupSnapshot() (backupItem, error) {
	name := "backup_" + time.Now().Format("20060102_150405")
	target := filepath.Join(s.backupDir(), name)
	if err := os.MkdirAll(target, 0755); err != nil {
		return backupItem{}, err
	}
	if err := copySnapshotTree(s.cfg.DataDir, target); err != nil {
		return backupItem{}, err
	}
	return backupItem{Name: name, CreatedAt: time.Now()}, nil
}

func (s *Server) handleListBackups(c *gin.Context) {
	items, err := listBackupItems(s.backupDir())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleCreateBackup(c *gin.Context) {
	item, err := s.createBackupSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "item": item, "message": "備份已建立"})
}

func (s *Server) handleRestoreBackup(c *gin.Context) {
	var req backupRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少備份名稱"})
		return
	}

	src := filepath.Join(s.backupDir(), name)
	if _, err := os.Stat(src); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "找不到指定備份"})
		return
	}

	if err := copySnapshotTree(src, s.cfg.DataDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.profiles.Load(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "資料已還原，但重新載入角色設定失敗"})
		return
	}
	_ = s.store.Load()
	_ = s.history.Load()
	_ = s.rules.Load()
	_ = s.relationships.Load()
	_ = s.timeline.Load()
	_ = s.foreshadow.Load()

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "備份已還原，建議重新索引"})
}

func addZipFile(zw *zip.Writer, path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeSection(sb *strings.Builder, title string, lines []string) {
	sb.WriteString("## " + title + "\n\n")
	if len(lines) == 0 {
		sb.WriteString("無。\n\n")
		return
	}
	for _, line := range lines {
		sb.WriteString("- " + line + "\n")
	}
	sb.WriteString("\n")
}

func (s *Server) buildChapterBundleMarkdown(name string) (string, error) {
	file, err := s.loadChapterFile(name)
	if err != nil {
		return "", err
	}
	chapterNo := chapterNumberFromName(file.Name)

	var sb strings.Builder
	sb.WriteString("# 章節完整報告\n\n")
	sb.WriteString(fmt.Sprintf("**章節：** %s\n\n", file.Title))
	sb.WriteString(fmt.Sprintf("**匯出時間：** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	sb.WriteString("## 原始章節\n\n```text\n")
	sb.WriteString(strings.TrimSpace(file.Content))
	sb.WriteString("\n```\n\n")

	historyLines := make([]string, 0)
	for _, entry := range s.history.Recent(0) {
		match := entry.ChapterFile == file.Name || entry.ChapterTitle == file.Title
		if !match {
			continue
		}
		line := fmt.Sprintf("%s・%s", historyEntryLabel(entry), entry.CreatedAt.Format("2006-01-02 15:04:05"))
		if entry.RewriteMode != "" {
			line += "・模式：" + entry.RewriteMode
		}
		historyLines = append(historyLines, line)
	}
	writeSection(&sb, "審查與修稿歷史", historyLines)

	timelineLines := make([]string, 0)
	for _, event := range s.timeline.GetSorted() {
		if event.Chapter != chapterNo {
			continue
		}
		timelineLines = append(timelineLines, fmt.Sprintf("%s：%s", event.Scene, event.Description))
	}
	writeSection(&sb, "時間軸", timelineLines)

	foreshadowLines := make([]string, 0)
	for _, item := range s.foreshadow.GetAll() {
		if item.Chapter != chapterNo && item.PlantedIn != file.Title {
			continue
		}
		foreshadowLines = append(foreshadowLines, fmt.Sprintf("%s（%s）", item.Description, item.Status))
	}
	writeSection(&sb, "伏筆", foreshadowLines)

	chSignals := extractor.AnalyzeChapter(file.Content, s.profiles.AllNames())
	charSet := make(map[string]struct{}, len(chSignals.KnownCharacters))
	for _, name := range chSignals.KnownCharacters {
		charSet[name] = struct{}{}
	}
	relationshipLines := make([]string, 0)
	for _, rel := range s.relationships.GetAll() {
		if _, ok := charSet[rel.From]; !ok {
			if _, ok := charSet[rel.To]; !ok {
				continue
			}
		}
		relationshipLines = append(relationshipLines, fmt.Sprintf("%s ↔ %s：%s", rel.From, rel.To, rel.Status))
	}
	writeSection(&sb, "角色關係", relationshipLines)
	return sb.String(), nil
}

func (s *Server) handleExportChapterBundle(c *gin.Context) {
	var req chapterReportRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	report, err := s.buildChapterBundleMarkdown(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	reportName := strings.TrimSuffix(req.Name, filepath.Ext(req.Name)) + "_bundle.md"
	w, err := zw.Create(reportName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := io.WriteString(w, report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if file, err := s.loadChapterFile(req.Name); err == nil {
		_ = addZipFile(zw, filepath.Join(s.chapterDir(), file.Name), filepath.Join("chapters", file.Name))
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.TrimSuffix(req.Name, filepath.Ext(req.Name))+"_bundle.zip"))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func filterScenesForExport(scenes []Scene, wanted []string) []Scene {
	if len(wanted) == 0 {
		return scenes
	}

	keep := make(map[string]struct{}, len(wanted))
	for _, title := range wanted {
		title = strings.TrimSpace(title)
		if title != "" {
			keep[title] = struct{}{}
		}
	}

	filtered := make([]Scene, 0, len(scenes))
	for _, scene := range scenes {
		if _, ok := keep[scene.Title]; ok {
			filtered = append(filtered, scene)
		}
	}
	return filtered
}

func rebuildChapterFromScenes(scenes []Scene) string {
	if len(scenes) == 0 {
		return ""
	}

	parts := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		parts = append(parts, "## "+scene.Title+"\n"+scene.Content)
	}
	return strings.Join(parts, "\n\n")
}

func rebuildChapterSelection(content string, scenes []Scene) string {
	if len(scenes) == 0 {
		return ""
	}

	parts := make([]string, 0, len(scenes)+1)
	if preamble := chapterPreamble(content); preamble != "" {
		parts = append(parts, preamble)
	}
	parts = append(parts, rebuildChapterFromScenes(scenes))
	return strings.Join(parts, "\n\n")
}

func (s *Server) buildSceneMetadataComment(chapterName string, scenes []Scene) string {
	if len(scenes) == 0 {
		return ""
	}

	plans, err := s.loadScenePlans(chapterName)
	if err != nil || len(plans) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<!-- manuscript-metadata\n")

	wroteScene := false
	for _, scene := range scenes {
		plan, ok := plans[scene.Title]
		if !ok {
			continue
		}

		hasFields := false
		var sceneSB strings.Builder
		sceneSB.WriteString("### ")
		sceneSB.WriteString(scene.Title)
		sceneSB.WriteString("\n")
		if plan.Synopsis != "" {
			sceneSB.WriteString("- 摘要：" + plan.Synopsis + "\n")
			hasFields = true
		}
		if plan.POV != "" {
			sceneSB.WriteString("- POV：" + plan.POV + "\n")
			hasFields = true
		}
		if plan.Conflict != "" {
			sceneSB.WriteString("- 衝突：" + plan.Conflict + "\n")
			hasFields = true
		}
		if plan.Purpose != "" {
			sceneSB.WriteString("- 目的：" + plan.Purpose + "\n")
			hasFields = true
		}
		if !hasFields {
			continue
		}
		sceneSB.WriteString("\n")
		sb.WriteString(sceneSB.String())
		wroteScene = true
	}

	if !wroteScene {
		return ""
	}

	sb.WriteString("-->")
	return sb.String()
}

func (s *Server) buildManuscriptMarkdown(req manuscriptExportRequest) (string, error) {
	files, err := s.listChapterFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("目前沒有章節可匯出")
	}

	selected := make(map[string]manuscriptExportSelection, len(req.Selections))
	for _, item := range req.Selections {
		normalized, err := normalizeChapterName(item.Name)
		if err != nil {
			continue
		}
		item.Name = normalized
		selected[normalized] = item
	}

	var sb strings.Builder
	sb.WriteString("# 小說手稿匯出\n\n")

	for _, item := range files {
		selection, selectedThisChapter := selected[item.Name]
		if len(selected) > 0 && !selectedThisChapter {
			continue
		}

		file, err := s.loadChapterFile(item.Name)
		if err != nil {
			return "", err
		}
		sceneOrder, err := s.loadScenePlanOrder(item.Name)
		if err != nil {
			return "", err
		}

		content := strings.TrimSpace(file.Content)
		exportScenes := orderedScenes(file.Scenes, sceneOrder)
		if len(file.Scenes) > 0 && len(selection.Scenes) > 0 {
			filteredScenes := filterScenesForExport(exportScenes, selection.Scenes)
			if len(filteredScenes) != len(selection.Scenes) {
				return "", fmt.Errorf("找不到指定場景：%s", strings.Join(selection.Scenes, "、"))
			}
			exportScenes = filteredScenes
		}
		if len(file.Scenes) > 0 && len(selection.Scenes) > 0 {
			if len(exportScenes) == 0 {
				continue
			}
			content = rebuildChapterSelection(file.Content, exportScenes)
		} else if len(file.Scenes) > 0 {
			content = rebuildChapterWithSceneOrder(file.Content, file.Scenes, sceneOrder)
		}

		sb.WriteString("## ")
		sb.WriteString(file.Title)
		sb.WriteString("\n\n")
		sb.WriteString(strings.TrimSpace(content))
		sb.WriteString("\n\n")

		if req.IncludeMetadata {
			if metadata := s.buildSceneMetadataComment(item.Name, exportScenes); metadata != "" {
				sb.WriteString(metadata)
				sb.WriteString("\n\n")
			}
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "# 小說手稿匯出" {
		return "", fmt.Errorf("沒有符合條件的章節或場景可匯出")
	}
	return result + "\n", nil
}

func (s *Server) handleExportManuscript(c *gin.Context) {
	var req manuscriptExportRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	markdown, err := s.buildManuscriptMarkdown(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.Format {
	case "html":
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="novel_manuscript.html"`)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(exporter.ManuscriptToHTML(markdown)))
	default:
		c.Header("Content-Type", "text/markdown; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="novel_manuscript.md"`)
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(markdown))
	}
}
