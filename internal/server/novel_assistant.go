package server

import (
	"net/http"

	"novel-assistant/internal/tracker"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleNovelAssistantPage(c *gin.Context) {
	chapters, err := s.buildChapterOverviews()
	if err != nil {
		chapters = nil
	}

	totalWords := 0
	reviewedCount := 0
	for _, ch := range chapters {
		totalWords += ch.WordCount
		if ch.ReviewCount > 0 {
			reviewedCount++
		}
	}

	var openFS, closedFS []tracker.Foreshadowing
	for _, f := range s.foreshadow.GetAll() {
		if f.Status == "已回收" {
			closedFS = append(closedFS, *f)
		} else {
			openFS = append(openFS, *f)
		}
	}

	c.HTML(http.StatusOK, "novel-assistant.html", gin.H{
		"Title":             "小說助手",
		"Chapters":          chapters,
		"ChapterCount":      len(chapters),
		"TotalWords":        totalWords,
		"ReviewedCount":     reviewedCount,
		"Characters":        s.profiles.Characters,
		"Worlds":            s.profiles.Worlds,
		"Styles":            s.profiles.Styles,
		"Timeline":          s.timeline.GetSorted(),
		"OpenForeshadows":   openFS,
		"ClosedForeshadows": closedFS,
		"Relationships":     s.relationships.GetAll(),
		"History":           s.history.Recent(20),
	})
}
