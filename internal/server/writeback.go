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
	SceneIndex   int      `json:"scene_index,omitempty"`
	Scene        string   `json:"scene"`
	Description  string   `json:"description"`
	Characters   []string `json:"characters"`
	Consequences string   `json:"consequences"`
}

type foreshadowWritebackRequest struct {
	Chapter     int    `json:"chapter"`
	SceneIndex  int    `json:"scene_index,omitempty"`
	Description string `json:"description"`
	PlantedIn   string `json:"planted_in"`
}

type relationshipWritebackRequest struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Status       string `json:"status"`
	Note         string `json:"note"`
	TriggerEvent string `json:"trigger_event"`
	Chapter      int    `json:"chapter,omitempty"`
	SceneIndex   int    `json:"scene_index,omitempty"`
}

// applyStateGraphDelta fires-and-forgets a state graph delta; errors are logged only.
func (s *Server) applyStateGraphDelta(chapter int, delta tracker.StateDelta) {
	if s.stateGraph == nil {
		return
	}
	if err := s.stateGraph.Apply(chapter, delta); err != nil {
		log.Printf("stategraph apply: %v", err)
		return
	}
	if err := s.stateGraph.Save(); err != nil {
		log.Printf("stategraph save: %v", err)
	}
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

	ev := &tracker.TimelineEvent{
		Chapter:      req.Chapter,
		SceneIndex:   req.SceneIndex,
		Scene:        strings.TrimSpace(req.Scene),
		Description:  strings.TrimSpace(req.Description),
		Characters:   req.Characters,
		Consequences: strings.TrimSpace(req.Consequences),
	}
	s.timeline.Add(ev)
	if !saveTrackerAsJSON(c, "save timeline", s.timeline.Save()) {
		return
	}
	s.applyStateGraphDelta(ev.Chapter, tracker.StateDelta{Events: []tracker.TimelineEvent{*ev}})
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

	fs := &tracker.Foreshadowing{
		Chapter:     req.Chapter,
		SceneIndex:  req.SceneIndex,
		Description: strings.TrimSpace(req.Description),
		PlantedIn:   strings.TrimSpace(req.PlantedIn),
	}
	s.foreshadow.Add(fs)
	if !saveTrackerAsJSON(c, "save foreshadow", s.foreshadow.Save()) {
		return
	}
	s.applyStateGraphDelta(fs.Chapter, tracker.StateDelta{AddedFS: []string{fs.ID}})
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

	rel := &tracker.Relationship{
		From:         strings.TrimSpace(req.From),
		To:           strings.TrimSpace(req.To),
		Status:       strings.TrimSpace(req.Status),
		Note:         strings.TrimSpace(req.Note),
		TriggerEvent: strings.TrimSpace(req.TriggerEvent),
		Chapter:      req.Chapter,
		SceneIndex:   req.SceneIndex,
	}
	s.relationships.Upsert(rel)
	if !saveTrackerAsJSON(c, "save relationship", s.relationships.Save()) {
		return
	}
	s.applyStateGraphDelta(rel.Chapter, tracker.StateDelta{Relationships: []tracker.RelationshipEdge{
		{From: rel.From, To: rel.To, Status: rel.Status, Note: rel.Note},
	}})
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已更新角色關係"})
}
