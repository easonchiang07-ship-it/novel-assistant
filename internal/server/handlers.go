package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"novel-assistant/internal/checker"
	"novel-assistant/internal/exporter"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/tracker"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type chanWriter struct{ ch chan<- string }

func (cw *chanWriter) Write(p []byte) (n int, err error) {
	cw.ch <- string(p)
	return len(p), nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func parsePositiveChapter(raw string) (int, error) {
	chapter, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || chapter < 1 {
		return 0, fmt.Errorf("章節必須是大於 0 的整數")
	}
	return chapter, nil
}

func saveOrAbort(c *gin.Context, err error, action string) bool {
	if err == nil {
		return true
	}
	log.Printf("%s: %v", action, err)
	c.String(http.StatusInternalServerError, "%s", "資料保存失敗，請稍後再試")
	return false
}

func joinProfiles(items []vectorProfile) string {
	if len(items) == 0 {
		return ""
	}

	var parts []string
	for _, item := range items {
		parts = append(parts, item.Content)
	}
	return strings.Join(parts, "\n\n")
}

type vectorProfile struct {
	Name    string
	Type    string
	Content string
}

func (s *Server) resolveStyles(req checkRequest) ([]*profile.StyleGuide, error) {
	if !contains(req.Checks, "style") {
		return nil, nil
	}
	if len(s.profiles.Styles) == 0 {
		return nil, fmt.Errorf("尚無可用的寫作風格設定，請先在 data/style/ 新增 .md 檔並重新索引")
	}

	names := req.Styles
	if len(names) == 0 {
		names = s.profiles.AllStyleNames()
	}

	styles := make([]*profile.StyleGuide, 0, len(names))
	for _, styleName := range names {
		sg := s.profiles.FindStyleByName(styleName)
		if sg == nil {
			return nil, fmt.Errorf("找不到寫作風格：%s", styleName)
		}
		if strings.TrimSpace(sg.RawContent) == "" {
			return nil, fmt.Errorf("寫作風格內容不可為空：%s", styleName)
		}
		styles = append(styles, sg)
	}
	return styles, nil
}

func (s *Server) buildReferenceContext(ctx context.Context, chapter string) ([]vectorProfile, error) {
	if s.store.Len() == 0 {
		return nil, nil
	}

	queryVec, err := s.embedder.Embed(ctx, chapter)
	if err != nil {
		return nil, err
	}

	docs := s.store.Query(queryVec, 4, "")
	results := make([]vectorProfile, 0, len(docs))
	for _, doc := range docs {
		name := strings.TrimPrefix(doc.ID, "char_")
		name = strings.TrimPrefix(name, "world_")
		results = append(results, vectorProfile{
			Name:    name,
			Type:    doc.Type,
			Content: doc.Content,
		})
	}
	return results, nil
}

func (s *Server) resolveCharacters(req checkRequest) []*profile.Character {
	names := req.Characters
	if len(names) == 0 {
		names = checker.ExtractNames(req.Chapter, s.profiles.AllNames())
	}
	if len(names) == 0 {
		names = s.profiles.AllNames()
	}

	chars := make([]*profile.Character, 0, len(names))
	for _, charName := range names {
		if char := s.profiles.FindByName(charName); char != nil {
			chars = append(chars, char)
		}
	}
	return chars
}

// ─── pages ───────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(c *gin.Context) {
	open := 0
	for _, f := range s.foreshadow.GetAll() {
		if f.Status == "未回收" {
			open++
		}
	}
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Title":          "儀表板",
		"CharCount":      len(s.profiles.Characters),
		"WorldCount":     len(s.profiles.Worlds),
		"RelCount":       len(s.relationships.GetAll()),
		"EventCount":     len(s.timeline.GetSorted()),
		"ForeshadowOpen": open,
		"VectorCount":    s.store.Len(),
	})
}

func (s *Server) handleCharacters(c *gin.Context) {
	c.HTML(http.StatusOK, "characters.html", gin.H{
		"Title":      "角色管理",
		"Characters": s.profiles.Characters,
		"Worlds":     s.profiles.Worlds,
	})
}

func (s *Server) handleCheckPage(c *gin.Context) {
	c.HTML(http.StatusOK, "check.html", gin.H{
		"Title":      "一致性審查",
		"Characters": s.profiles.Characters,
		"Styles":     s.profiles.Styles,
	})
}

func (s *Server) handleRelationshipsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "relationships.html", gin.H{
		"Title":         "關係圖",
		"Relationships": s.relationships.GetAll(),
		"Characters":    s.profiles.AllNames(),
	})
}

func (s *Server) handleTimelinePage(c *gin.Context) {
	c.HTML(http.StatusOK, "timeline.html", gin.H{
		"Title":      "時間軸",
		"Events":     s.timeline.GetSorted(),
		"Characters": s.profiles.AllNames(),
	})
}

func (s *Server) handleForeshadowPage(c *gin.Context) {
	c.HTML(http.StatusOK, "foreshadow.html", gin.H{
		"Title":       "伏筆追蹤",
		"Foreshadows": s.foreshadow.GetAll(),
	})
}

// ─── ingest ───────────────────────────────────────────────────────────────────

func (s *Server) handleIngest(c *gin.Context) {
	ctx := c.Request.Context()
	if err := s.Ingest(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":    "索引完成",
		"characters": len(s.profiles.Characters),
		"worlds":     len(s.profiles.Worlds),
	})
}

// ─── check stream ─────────────────────────────────────────────────────────────

type checkRequest struct {
	Chapter    string   `json:"chapter"`
	Characters []string `json:"characters"`
	Checks     []string `json:"checks"` // ["behavior","dialogue","style"]
	Styles     []string `json:"styles"` // 指定風格名稱；空白 = 所有風格
}

func (s *Server) handleCheckStream(c *gin.Context) {
	var req checkRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	if _, err := s.resolveStyles(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msgChan := make(chan string, 512)
	ctx, cancel := context.WithCancel(c.Request.Context())

	go func() {
		defer close(msgChan)
		defer cancel()

		cw := &chanWriter{ch: msgChan}
		charsToCheck := s.resolveCharacters(req)
		if len(charsToCheck) == 0 {
			msgChan <- "\n> 找不到可審查的角色，請先建立角色設定檔。\n"
			return
		}

		references, err := s.buildReferenceContext(ctx, req.Chapter)
		if err != nil {
			msgChan <- fmt.Sprintf("\n> RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
		} else if len(references) > 0 {
			msgChan <- "### 本地參考上下文\n\n"
			for _, ref := range references {
				msgChan <- fmt.Sprintf("- [%s] %s\n", ref.Type, ref.Name)
			}
			msgChan <- "\n"
		}

		referenceText := joinProfiles(references)
		for _, char := range charsToCheck {
			msgChan <- fmt.Sprintf("\n\n## 角色：%s\n\n", char.Name)

			if len(req.Checks) == 0 || contains(req.Checks, "behavior") {
				msgChan <- "### 行為一致性審查\n\n"
				profileText := char.RawContent
				if referenceText != "" {
					profileText += "\n\n【補充參考資料】\n" + referenceText
				}
				if err := s.checker.CheckBehaviorStream(ctx, profileText, req.Chapter, cw); err != nil {
					if ctx.Err() == nil {
						msgChan <- fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
					}
					return
				}
			}

			if contains(req.Checks, "dialogue") {
				msgChan <- "\n\n### 對白風格審查\n\n"
				if err := s.checker.CheckDialogueStream(ctx, char.Name, char.Personality, char.SpeechStyle, req.Chapter, cw); err != nil {
					if ctx.Err() == nil {
						msgChan <- fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
					}
					return
				}
			}
		}

		// 寫作風格審查（獨立於角色，逐一套用所選風格）
		stylesToCheck, err := s.resolveStyles(req)
		if err != nil {
			msgChan <- fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
			return
		}
		for _, sg := range stylesToCheck {
			msgChan <- fmt.Sprintf("\n\n## 寫作風格：%s\n\n### 風格一致性審查\n\n", sg.Name)
			if err := s.checker.CheckStyleStream(ctx, sg.RawContent, req.Chapter, cw); err != nil {
				if ctx.Err() == nil {
					msgChan <- fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
				}
				return
			}
		}

		msgChan <- "\n\n---\n審查完成\n"
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return false
			}
			c.SSEvent("chunk", gin.H{"text": msg})
			return true
		case <-ctx.Done():
			return false
		}
	})
}

// ─── relationships ────────────────────────────────────────────────────────────

func (s *Server) handleAddRelationship(c *gin.Context) {
	r := &tracker.Relationship{
		From:         c.PostForm("from"),
		To:           c.PostForm("to"),
		Status:       c.PostForm("status"),
		Note:         c.PostForm("note"),
		TriggerEvent: c.PostForm("trigger_event"),
	}
	s.relationships.Upsert(r)
	if !saveOrAbort(c, s.relationships.Save(), "save relationships") {
		return
	}
	c.Redirect(http.StatusFound, "/relationships")
}

func (s *Server) handleDeleteRelationship(c *gin.Context) {
	s.relationships.Delete(c.PostForm("from"), c.PostForm("to"))
	if !saveOrAbort(c, s.relationships.Save(), "save relationships") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── timeline ─────────────────────────────────────────────────────────────────

func (s *Server) handleAddTimelineEvent(c *gin.Context) {
	chapter, err := parsePositiveChapter(c.PostForm("chapter"))
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	var chars []string
	for _, ch := range strings.Split(c.PostForm("characters"), ",") {
		if t := strings.TrimSpace(ch); t != "" {
			chars = append(chars, t)
		}
	}
	s.timeline.Add(&tracker.TimelineEvent{
		Chapter:      chapter,
		Scene:        c.PostForm("scene"),
		Description:  c.PostForm("description"),
		Characters:   chars,
		Consequences: c.PostForm("consequences"),
	})
	if !saveOrAbort(c, s.timeline.Save(), "save timeline") {
		return
	}
	c.Redirect(http.StatusFound, "/timeline")
}

func (s *Server) handleDeleteTimelineEvent(c *gin.Context) {
	s.timeline.Delete(c.PostForm("id"))
	if !saveOrAbort(c, s.timeline.Save(), "save timeline") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── foreshadow ───────────────────────────────────────────────────────────────

func (s *Server) handleAddForeshadow(c *gin.Context) {
	chapter, err := parsePositiveChapter(c.PostForm("chapter"))
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	s.foreshadow.Add(&tracker.Foreshadowing{
		Chapter:     chapter,
		Description: c.PostForm("description"),
		PlantedIn:   c.PostForm("planted_in"),
	})
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.Redirect(http.StatusFound, "/foreshadow")
}

func (s *Server) handleResolveForeshadow(c *gin.Context) {
	s.foreshadow.Resolve(c.PostForm("id"), c.PostForm("resolved_in"))
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.Redirect(http.StatusFound, "/foreshadow")
}

func (s *Server) handleDeleteForeshadow(c *gin.Context) {
	s.foreshadow.Delete(c.PostForm("id"))
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── export ───────────────────────────────────────────────────────────────────

func (s *Server) handleExport(c *gin.Context) {
	path, err := exporter.ExportMarkdown(
		c.PostForm("title"),
		c.PostForm("chapter"),
		c.PostForm("content"),
		s.cfg.DataDir+"/exports",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.FileAttachment(path, fmt.Sprintf("report_%s.md", c.PostForm("chapter")))
}
