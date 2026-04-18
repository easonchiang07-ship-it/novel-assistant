package server

import (
	"log"
	"net/http"
	"strings"

	"novel-assistant/internal/tracker"

	"github.com/gin-gonic/gin"
)

type timelineWritebackRequest struct {
	Chapter      int      `json:"chapter"`
	Scene        string   `json:"scene"`
	Description  string   `json:"description"`
	Characters   []string `json:"characters"`
	Consequences string   `json:"consequences"`
}

type foreshadowWritebackRequest struct {
	Chapter     int    `json:"chapter"`
	Description string `json:"description"`
	PlantedIn   string `json:"planted_in"`
}

type relationshipWritebackRequest struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Status       string `json:"status"`
	Note         string `json:"note"`
	TriggerEvent string `json:"trigger_event"`
}

func saveTrackerAsJSON(c *gin.Context, action string, err error) bool {
	if err == nil {
		return true
	}
	log.Printf("%s: %v", action, err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "資料保存失敗，請稍後再試"})
	return false
}

func (s *Server) handleWritebackTimeline(c *gin.Context) {
	var req timelineWritebackRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Chapter < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節必須是大於 0 的整數"})
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "時間軸描述不可為空"})
		return
	}

	s.timeline.Add(&tracker.TimelineEvent{
		Chapter:      req.Chapter,
		Scene:        strings.TrimSpace(req.Scene),
		Description:  strings.TrimSpace(req.Description),
		Characters:   req.Characters,
		Consequences: strings.TrimSpace(req.Consequences),
	})
	if !saveTrackerAsJSON(c, "save timeline", s.timeline.Save()) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已加入時間軸"})
}

func (s *Server) handleWritebackForeshadow(c *gin.Context) {
	var req foreshadowWritebackRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Chapter < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節必須是大於 0 的整數"})
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "伏筆描述不可為空"})
		return
	}

	s.foreshadow.Add(&tracker.Foreshadowing{
		Chapter:     req.Chapter,
		Description: strings.TrimSpace(req.Description),
		PlantedIn:   strings.TrimSpace(req.PlantedIn),
	})
	if !saveTrackerAsJSON(c, "save foreshadow", s.foreshadow.Save()) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已加入伏筆追蹤"})
}

func (s *Server) handleWritebackRelationship(c *gin.Context) {
	var req relationshipWritebackRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.From) == "" || strings.TrimSpace(req.To) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色關係需要選擇雙方角色"})
		return
	}
	if strings.TrimSpace(req.Status) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請選擇關係狀態"})
		return
	}

	s.relationships.Upsert(&tracker.Relationship{
		From:         strings.TrimSpace(req.From),
		To:           strings.TrimSpace(req.To),
		Status:       strings.TrimSpace(req.Status),
		Note:         strings.TrimSpace(req.Note),
		TriggerEvent: strings.TrimSpace(req.TriggerEvent),
	})
	if !saveTrackerAsJSON(c, "save relationship", s.relationships.Save()) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已更新角色關係"})
}
