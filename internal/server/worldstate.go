package server

import (
	"fmt"
	"net/http"
	"strings"

	"novel-assistant/internal/worldstate"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleCreateWorldstateSnapshot(c *gin.Context) {
	file, err := s.loadChapterFile(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(file.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}

	changes, err := s.checker.GenerateWorldStateChanges(c.Request.Context(), file.Content)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	snapshot := &worldstate.Snapshot{
		ChapterFile:  file.Name,
		ChapterIndex: chapterNumberFromName(file.Name),
		Changes:      changes,
	}
	s.worldstate.Upsert(snapshot)
	if err := s.worldstate.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	latest := s.worldstate.GetByChapterFile(file.Name)
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"message":  "已產生章節狀態快照",
		"snapshot": latest,
	})
}

func (s *Server) handleListWorldstate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": s.worldstate.GetAll()})
}

func (s *Server) worldStateSystemPrefix(chapterFile string) string {
	chapterFile = strings.TrimSpace(chapterFile)
	if chapterFile == "" || s.worldstate == nil {
		return ""
	}
	chapterIndex := chapterNumberFromName(chapterFile)
	if chapterIndex <= 0 {
		return ""
	}
	snapshot := s.worldstate.GetLatestBefore(chapterIndex)
	if snapshot == nil || len(snapshot.Changes) == 0 {
		return ""
	}

	lines := make([]string, 0, len(snapshot.Changes))
	for _, change := range snapshot.Changes {
		description := strings.TrimSpace(change.Description)
		entity := strings.TrimSpace(change.Entity)
		switch {
		case entity != "" && description != "":
			lines = append(lines, fmt.Sprintf("- %s：%s", entity, description))
		case description != "":
			lines = append(lines, "- "+description)
		case entity != "":
			lines = append(lines, "- "+entity)
		}
	}
	if len(lines) == 0 {
		return ""
	}

	return fmt.Sprintf("【當前世界狀態（截至第 %d 章）】\n%s", snapshot.ChapterIndex, strings.Join(lines, "\n"))
}

func summarizeSnapshot(snapshot *worldstate.Snapshot) []string {
	if snapshot == nil {
		return nil
	}
	lines := make([]string, 0, len(snapshot.Changes))
	for _, change := range snapshot.Changes {
		description := strings.TrimSpace(change.Description)
		entity := strings.TrimSpace(change.Entity)
		switch {
		case entity != "" && description != "":
			lines = append(lines, fmt.Sprintf("%s：%s", entity, description))
		case description != "":
			lines = append(lines, description)
		case entity != "":
			lines = append(lines, entity)
		}
	}
	return lines
}
