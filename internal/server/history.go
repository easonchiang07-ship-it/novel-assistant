package server

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"novel-assistant/internal/diffview"
	"novel-assistant/internal/reviewhistory"

	"github.com/gin-gonic/gin"
)

type historyGroup struct {
	ChapterTitle string
	Entries      []*reviewhistory.Entry
}

type historyDeleteRequest struct {
	ID string `json:"id"`
}

type historyExportRequest struct {
	IDs []string `json:"ids"`
}

func historyEntryLabel(entry *reviewhistory.Entry) string {
	if entry == nil {
		return ""
	}
	switch entry.Kind {
	case "rewrite":
		return fmt.Sprintf("第 %d 次修稿", entry.KindVersion)
	default:
		return fmt.Sprintf("第 %d 次審查", entry.KindVersion)
	}
}

func historyEditorContent(entry *reviewhistory.Entry) string {
	if entry == nil {
		return ""
	}
	content := strings.TrimSpace(entry.Result)
	content = strings.TrimSuffix(content, "修稿完成")
	content = strings.TrimSpace(content)
	content = strings.TrimSuffix(content, "---")
	return strings.TrimSpace(content)
}

func historySourceContent(entry *reviewhistory.Entry) string {
	if entry == nil {
		return ""
	}
	return strings.TrimSpace(entry.InputContent)
}

func formatRetrievalConfigMap(items map[string]reviewhistory.RetrievalConfig) string {
	if len(items) == 0 {
		return ""
	}

	order := []string{"behavior", "dialogue", "world", "rewrite"}
	labels := map[string]string{
		"behavior":  "行為",
		"dialogue":  "對白",
		"world":     "世界觀",
		"rewrite":   "修稿",
		"chapter":   "章節脈絡",
		"character": "角色",
		"style":     "風格",
	}

	parts := make([]string, 0, len(items))
	for _, key := range order {
		cfg, ok := items[key]
		if !ok || cfg.TopK == 0 {
			continue
		}
		sourceLabels := make([]string, 0, len(cfg.Sources))
		for _, source := range cfg.Sources {
			if label, ok := labels[source]; ok {
				sourceLabels = append(sourceLabels, label)
			} else {
				sourceLabels = append(sourceLabels, source)
			}
		}
		taskLabel := labels[key]
		if taskLabel == "" {
			taskLabel = key
		}
		parts = append(parts, fmt.Sprintf("%s：%s / Top-K %d / 門檻 %.2f", taskLabel, strings.Join(sourceLabels, "、"), cfg.TopK, cfg.Threshold))
	}
	return strings.Join(parts, "；")
}

func buildHistoryGroups(entries []*reviewhistory.Entry) []historyGroup {
	order := make([]string, 0)
	groups := make(map[string][]*reviewhistory.Entry)

	for _, entry := range entries {
		title := strings.TrimSpace(entry.ChapterTitle)
		if title == "" {
			title = "未命名章節"
		}
		if _, ok := groups[title]; !ok {
			order = append(order, title)
		}
		groups[title] = append(groups[title], entry)
	}

	result := make([]historyGroup, 0, len(order))
	for _, title := range order {
		result = append(result, historyGroup{
			ChapterTitle: title,
			Entries:      groups[title],
		})
	}
	return result
}

func formatHistoryMarkdown(entries []*reviewhistory.Entry) []byte {
	var buf bytes.Buffer
	buf.WriteString("# 審查歷史匯出\n\n")
	buf.WriteString(fmt.Sprintf("共 %d 筆紀錄。\n\n", len(entries)))

	for _, entry := range entries {
		buf.WriteString(fmt.Sprintf("## %s\n\n", entry.ChapterTitle))
		if entry.Kind == "rewrite" {
			buf.WriteString("- 類型：修稿模式\n")
		} else {
			buf.WriteString("- 類型：審查\n")
		}
		if entry.ChapterVersion > 0 {
			buf.WriteString(fmt.Sprintf("- 章節版本序號：第 %d 筆\n", entry.ChapterVersion))
		}
		if entry.KindVersion > 0 {
			buf.WriteString(fmt.Sprintf("- 類型版本序號：%s\n", historyEntryLabel(entry)))
		}
		if entry.RewriteMode != "" {
			buf.WriteString(fmt.Sprintf("- 修稿模式：%s\n", entry.RewriteMode))
		}
		if entry.ChapterFile != "" {
			buf.WriteString(fmt.Sprintf("- 章節檔案：%s\n", entry.ChapterFile))
		}
		if len(entry.Checks) > 0 {
			buf.WriteString(fmt.Sprintf("- 檢查項目：%s\n", strings.Join(entry.Checks, "、")))
		}
		if len(entry.Styles) > 0 {
			buf.WriteString(fmt.Sprintf("- 風格：%s\n", strings.Join(entry.Styles, "、")))
		}
		if len(entry.Sources) > 0 {
			buf.WriteString(fmt.Sprintf("- 參考來源：%s\n", strings.Join(entry.Sources, "、")))
		}
		if summary := formatRetrievalConfigMap(entry.RetrievalConfigs); summary != "" {
			buf.WriteString(fmt.Sprintf("- Retrieval 設定：%s\n", summary))
		}
		buf.WriteString(fmt.Sprintf("- 建立時間：%s\n\n", entry.CreatedAt.Format("2006-01-02 15:04:05")))
		buf.WriteString("```text\n")
		buf.WriteString(strings.TrimSpace(entry.Result))
		buf.WriteString("\n```\n\n")
	}

	return buf.Bytes()
}

func (s *Server) handleHistoryPage(c *gin.Context) {
	entries := s.history.Recent(50)
	c.HTML(http.StatusOK, "history.html", gin.H{
		"Title":      "審查歷史",
		"Entries":    entries,
		"EntryCount": len(entries),
		"Groups":     buildHistoryGroups(entries),
	})
}

func (s *Server) handleGetHistoryEntry(c *gin.Context) {
	entry := s.history.Find(c.Param("id"))
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定的歷史紀錄"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"item":           entry,
		"kind_label":     historyEntryLabel(entry),
		"editor_content": historyEditorContent(entry),
	})
}

func (s *Server) handleGetHistoryDiff(c *gin.Context) {
	entry := s.history.Find(c.Param("id"))
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定的歷史紀錄"})
		return
	}

	before := historySourceContent(entry)
	after := historyEditorContent(entry)
	if before == "" && entry.ChapterFile != "" {
		file, err := s.loadChapterFile(entry.ChapterFile)
		if err == nil {
			before = file.Content
		}
	}
	if before == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "這筆歷史紀錄沒有可比較的原文"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"item":     entry,
		"before":   before,
		"after":    after,
		"segments": diffview.LineDiff(before, after),
	})
}

func (s *Server) handleDeleteHistoryEntry(c *gin.Context) {
	var req historyDeleteRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少歷史紀錄 ID"})
		return
	}
	if !s.history.Delete(req.ID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定的歷史紀錄"})
		return
	}
	if err := s.history.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "歷史紀錄保存失敗"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "歷史紀錄已刪除"})
}

func (s *Server) handleExportHistory(c *gin.Context) {
	var req historyExportRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries := s.history.Select(req.IDs, 200)
	if len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "沒有可匯出的歷史紀錄"})
		return
	}

	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="review-history.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", formatHistoryMarkdown(entries))
}
