package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"novel-assistant/internal/exporter"
	"novel-assistant/internal/extractor"
	"novel-assistant/internal/reviewhistory"

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

type manuscriptAppendixOptions struct {
	Reviews bool `json:"reviews"`
	Tracker bool `json:"tracker"`
}

type manuscriptExportRequest struct {
	Selections      []manuscriptExportSelection `json:"selections"`
	IncludeMetadata bool                        `json:"include_metadata"`
	Appendix        manuscriptAppendixOptions   `json:"appendix"`
	Format          string                      `json:"format"`
}

type backupItem struct {
	Name      string        `json:"name"`
	CreatedAt time.Time     `json:"created_at"`
	Preview   backupPreview `json:"preview"`
}

type backupPreview struct {
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	ChapterCount   int       `json:"chapter_count"`
	CharacterCount int       `json:"character_count"`
	WorldCount     int       `json:"world_count"`
	StyleCount     int       `json:"style_count"`
	JSONFileCount  int       `json:"json_file_count"`
	TotalFileCount int       `json:"total_file_count"`
	ChapterSamples []string  `json:"chapter_samples"`
}

func (s *Server) backupDir() string {
	return filepath.Join(s.cfg.DataDir, "backups")
}

func backupManifestPath(dir string) string {
	return filepath.Join(dir, ".backup_manifest.json")
}

func loadBackupPreview(dir string, fallbackName string, fallbackTime time.Time) (backupPreview, error) {
	data, err := os.ReadFile(backupManifestPath(dir))
	if err == nil {
		var preview backupPreview
		if err := json.Unmarshal(data, &preview); err == nil {
			if preview.Name == "" {
				preview.Name = fallbackName
			}
			if preview.CreatedAt.IsZero() {
				preview.CreatedAt = fallbackTime
			}
			return preview, nil
		}
	}
	return buildBackupPreview(dir, fallbackName, fallbackTime)
}

func buildBackupPreview(dir string, name string, createdAt time.Time) (backupPreview, error) {
	preview := backupPreview{Name: name, CreatedAt: createdAt}
	chapterSamples := make([]string, 0, 3)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == ".backup_manifest.json" {
			return nil
		}

		preview.TotalFileCount++
		switch {
		case strings.HasPrefix(rel, "chapters"+string(filepath.Separator)) && strings.HasSuffix(strings.ToLower(d.Name()), ".md"):
			preview.ChapterCount++
			if len(chapterSamples) < 3 {
				chapterSamples = append(chapterSamples, d.Name())
			}
		case strings.HasPrefix(rel, "characters"+string(filepath.Separator)) && strings.HasSuffix(strings.ToLower(d.Name()), ".md"):
			preview.CharacterCount++
		case strings.HasPrefix(rel, "worldbuilding"+string(filepath.Separator)) && strings.HasSuffix(strings.ToLower(d.Name()), ".md"):
			preview.WorldCount++
		case strings.HasPrefix(rel, "style"+string(filepath.Separator)) && strings.HasSuffix(strings.ToLower(d.Name()), ".md"):
			preview.StyleCount++
		case strings.HasSuffix(strings.ToLower(d.Name()), ".json"):
			preview.JSONFileCount++
		}
		return nil
	})
	if err != nil {
		return backupPreview{}, err
	}
	sort.Strings(chapterSamples)
	preview.ChapterSamples = chapterSamples
	return preview, nil
}

func writeBackupPreview(dir string, preview backupPreview) error {
	data, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(backupManifestPath(dir), data, 0644)
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
		preview, err := loadBackupPreview(filepath.Join(dir, entry.Name()), entry.Name(), info.ModTime())
		if err != nil {
			continue
		}
		items = append(items, backupItem{Name: entry.Name(), CreatedAt: info.ModTime(), Preview: preview})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
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

// cleanDataDirForRestore removes all managed files (.md, .json, .gitkeep)
// from dataDir before a restore so that files absent from the snapshot are
// not left behind (true point-in-time restore, not an overlay).
func cleanDataDirForRestore(dataDir string) error {
	return filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dataDir {
			return nil
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		if rel == "backups" || strings.HasPrefix(rel, "backups"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext == ".md" || ext == ".json" || d.Name() == ".gitkeep" {
			return os.Remove(path)
		}
		return nil
	})
}

func pruneOldBackups(dir string, retain int, protected map[string]struct{}) ([]string, error) {
	if retain < 1 {
		return nil, nil
	}
	items, err := listBackupItems(dir)
	if err != nil {
		return nil, err
	}
	if len(items) <= retain {
		return nil, nil
	}

	removed := make([]string, 0, len(items)-retain)
	for _, item := range items[retain:] {
		if _, ok := protected[item.Name]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, item.Name)); err != nil {
			return removed, err
		}
		removed = append(removed, item.Name)
	}
	return removed, nil
}

// writeSnapshot creates a snapshot directory and manifest without pruning.
func (s *Server) writeSnapshot(prefix string) (backupItem, error) {
	name := prefix + "_" + time.Now().Format("20060102_150405")
	createdAt := time.Now()
	target := filepath.Join(s.backupDir(), name)
	if err := os.MkdirAll(target, 0755); err != nil {
		return backupItem{}, err
	}
	if err := copySnapshotTree(s.cfg.DataDir, target); err != nil {
		return backupItem{}, err
	}
	preview, err := buildBackupPreview(target, name, createdAt)
	if err != nil {
		return backupItem{}, err
	}
	if err := writeBackupPreview(target, preview); err != nil {
		return backupItem{}, err
	}
	return backupItem{Name: name, CreatedAt: createdAt, Preview: preview}, nil
}

func (s *Server) createBackupSnapshotWithPrefix(prefix string) (backupItem, []string, error) {
	return s.createBackupSnapshotWithPrefixProtected(prefix, nil)
}

func (s *Server) createBackupSnapshotWithPrefixProtected(prefix string, protected map[string]struct{}) (backupItem, []string, error) {
	item, err := s.writeSnapshot(prefix)
	if err != nil {
		return backupItem{}, nil, err
	}
	retention := 10
	if s.project != nil {
		retention = s.project.Get().BackupRetention
	}
	removed, err := pruneOldBackups(s.backupDir(), retention, protected)
	if err != nil {
		return backupItem{}, removed, err
	}
	return item, removed, nil
}

func (s *Server) createBackupSnapshot() (backupItem, error) {
	item, _, err := s.createBackupSnapshotWithPrefix("backup")
	return item, err
}

func (s *Server) handleListBackups(c *gin.Context) {
	items, err := listBackupItems(s.backupDir())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleGetBackupPreview(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少備份名稱"})
		return
	}
	src := filepath.Join(s.backupDir(), name)
	info, err := os.Stat(src)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定備份"})
		return
	}
	preview, err := loadBackupPreview(src, name, info.ModTime())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": backupItem{Name: name, CreatedAt: info.ModTime(), Preview: preview}})
}

func (s *Server) handleCreateBackup(c *gin.Context) {
	item, removed, err := s.createBackupSnapshotWithPrefix("backup")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	message := "備份已建立"
	if len(removed) > 0 {
		message += fmt.Sprintf("，並清理 %d 份舊備份", len(removed))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "item": item, "removed": removed, "message": message})
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

	safetySnapshot, err := s.writeSnapshot("pre_restore")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "建立還原前安全備份失敗：" + err.Error()})
		return
	}

	if err := cleanDataDirForRestore(s.cfg.DataDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除現有資料失敗：" + err.Error()})
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
	if s.project != nil {
		_ = s.project.Load()
		s.applyProjectSettings()
	}

	retention := 10
	if s.project != nil {
		retention = s.project.Get().BackupRetention
	}
	protected := map[string]struct{}{name: {}, safetySnapshot.Name: {}}
	removed, _ := pruneOldBackups(s.backupDir(), retention, protected)

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"message":         fmt.Sprintf("已還原備份 %s，並先建立安全備份 %s。建議重新索引。", name, safetySnapshot.Name),
		"restored":        name,
		"safety_snapshot": safetySnapshot,
		"removed":         removed,
	})
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

func normalizedSelectionMap(selections []manuscriptExportSelection) map[string]manuscriptExportSelection {
	selected := make(map[string]manuscriptExportSelection, len(selections))
	for _, item := range selections {
		normalized, err := normalizeChapterName(item.Name)
		if err != nil {
			continue
		}
		item.Name = normalized
		selected[normalized] = item
	}
	return selected
}

func chapterIncludedForAppendix(selected map[string]manuscriptExportSelection, chapterFile, chapterTitle string) bool {
	if len(selected) == 0 {
		return true
	}
	if chapterFile != "" {
		if normalized, err := normalizeChapterName(chapterFile); err == nil {
			if _, ok := selected[normalized]; ok {
				return true
			}
		}
	}
	if chapterTitle != "" {
		if normalized, err := normalizeChapterName(chapterTitle); err == nil {
			if _, ok := selected[normalized]; ok {
				return true
			}
		}
	}
	return false
}

func buildReviewAppendixLine(entry *reviewhistory.Entry) string {
	line := historyEntryLabel(entry)
	if !entry.CreatedAt.IsZero() {
		line += "・" + entry.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if entry.RewriteMode != "" {
		line += "・模式：" + entry.RewriteMode
	}
	if scene := strings.TrimSpace(entry.SceneTitle); scene != "" {
		line += "・場景：" + scene
	}
	return line
}

func writeAppendixListSection(sb *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	sb.WriteString("### ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	for _, line := range lines {
		sb.WriteString("- ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

type appendixChapterRef struct {
	Name  string
	Title string
}

func buildAppendixChapterRefs(files []chapterFile) map[int][]appendixChapterRef {
	refs := make(map[int][]appendixChapterRef, len(files))
	for _, file := range files {
		number := chapterNumberFromName(file.Name)
		refs[number] = append(refs[number], appendixChapterRef{
			Name:  file.Name,
			Title: file.Title,
		})
	}
	return refs
}

func resolveAppendixChapterRef(selected map[string]manuscriptExportSelection, refs map[int][]appendixChapterRef, chapter int, fallback string) (string, string) {
	if fallback != "" {
		if normalized, err := normalizeChapterName(fallback); err == nil {
			return normalized, chapterTitle(normalized)
		}
		return fallback, strings.TrimSpace(fallback)
	}

	candidates := refs[chapter]
	if len(candidates) == 0 {
		if chapter > 0 {
			name := fmt.Sprintf("第%02d章.md", chapter)
			return name, chapterTitle(name)
		}
		return "", ""
	}

	if len(selected) > 0 {
		selectedMatches := make([]appendixChapterRef, 0, len(candidates))
		for _, candidate := range candidates {
			if _, ok := selected[candidate.Name]; ok {
				selectedMatches = append(selectedMatches, candidate)
			}
		}
		if len(selectedMatches) == 1 {
			return selectedMatches[0].Name, selectedMatches[0].Title
		}
	}

	if len(candidates) == 1 {
		return candidates[0].Name, candidates[0].Title
	}
	return "", ""
}

func (s *Server) buildManuscriptAppendix(req manuscriptExportRequest, files []chapterFile) string {
	selected := normalizedSelectionMap(req.Selections)
	chapterRefs := buildAppendixChapterRefs(files)
	sections := make([]string, 0, 2)

	if req.Appendix.Reviews {
		groups := make(map[string][]string)
		order := make([]string, 0)
		for _, entry := range s.history.Recent(0) {
			if !chapterIncludedForAppendix(selected, entry.ChapterFile, entry.ChapterTitle) {
				continue
			}
			title := strings.TrimSpace(entry.ChapterTitle)
			if title == "" {
				switch {
				case entry.ChapterFile != "":
					title = chapterTitle(entry.ChapterFile)
				default:
					title = "未分類章節"
				}
			}
			if _, ok := groups[title]; !ok {
				order = append(order, title)
			}
			groups[title] = append(groups[title], buildReviewAppendixLine(entry))
		}
		if len(order) > 0 {
			var sb strings.Builder
			sb.WriteString("## 審查與修稿歷史\n\n")
			for _, title := range order {
				sb.WriteString("### ")
				sb.WriteString(title)
				sb.WriteString("\n\n")
				for _, line := range groups[title] {
					sb.WriteString("- ")
					sb.WriteString(line)
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
			sections = append(sections, strings.TrimSpace(sb.String()))
		}
	}

	if req.Appendix.Tracker {
		var sb strings.Builder
		sb.WriteString("## 追蹤資料\n\n")

		timelineLines := make([]string, 0)
		for _, event := range s.timeline.GetSorted() {
			chapterName, chapterTitle := resolveAppendixChapterRef(selected, chapterRefs, event.Chapter, "")
			if chapterName == "" || !chapterIncludedForAppendix(selected, chapterName, chapterTitle) {
				continue
			}
			timelineLines = append(timelineLines, fmt.Sprintf("%s：%s：%s", chapterTitle, event.Scene, event.Description))
		}
		writeAppendixListSection(&sb, "時間軸", timelineLines)

		foreshadowLines := make([]string, 0)
		for _, item := range s.foreshadow.GetAll() {
			chapterName, chapterTitle := resolveAppendixChapterRef(selected, chapterRefs, item.Chapter, item.PlantedIn)
			if chapterName == "" || !chapterIncludedForAppendix(selected, chapterName, chapterTitle) {
				continue
			}
			foreshadowLines = append(foreshadowLines, fmt.Sprintf("%s：%s（%s）", chapterTitle, item.Description, item.Status))
		}
		writeAppendixListSection(&sb, "伏筆", foreshadowLines)

		relationshipLines := make([]string, 0)
		for _, rel := range s.relationships.GetAll() {
			line := fmt.Sprintf("%s ↔ %s：%s", rel.From, rel.To, rel.Status)
			if note := strings.TrimSpace(rel.Note); note != "" {
				line += "・" + note
			}
			if trigger := strings.TrimSpace(rel.TriggerEvent); trigger != "" {
				line += "・事件：" + trigger
			}
			relationshipLines = append(relationshipLines, line)
		}
		writeAppendixListSection(&sb, "角色關係", relationshipLines)

		trackerSection := strings.TrimSpace(sb.String())
		if trackerSection != "## 追蹤資料" {
			sections = append(sections, trackerSection)
		}
	}

	if len(sections) == 0 {
		return ""
	}
	return "# 附錄\n\n" + strings.Join(sections, "\n\n") + "\n"
}

func (s *Server) buildManuscriptMarkdown(req manuscriptExportRequest) (string, error) {
	files, err := s.listChapterFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("目前沒有章節可匯出")
	}

	selected := normalizedSelectionMap(req.Selections)

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

	if appendix := strings.TrimSpace(s.buildManuscriptAppendix(req, files)); appendix != "" {
		sb.WriteString(appendix)
		sb.WriteString("\n")
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
