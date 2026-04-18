package server

import (
	"net/http"

	"novel-assistant/internal/reviewrules"

	"github.com/gin-gonic/gin"
)

type settingsSaveRequest struct {
	DefaultChecks []string `json:"default_checks"`
	DefaultStyles []string `json:"default_styles"`
	ReviewBias    string   `json:"review_bias"`
	RewriteBias   string   `json:"rewrite_bias"`
}

func (s *Server) handleSettingsPage(c *gin.Context) {
	settings := s.rules.Get()
	c.HTML(http.StatusOK, "settings.html", gin.H{
		"Title":         "規則設定",
		"Settings":      settings,
		"Styles":        s.profiles.Styles,
		"DefaultChecks": settings.DefaultChecks,
		"DefaultStyles": settings.DefaultStyles,
	})
}

func (s *Server) handleSaveSettings(c *gin.Context) {
	var req settingsSaveRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.rules.Update(reviewrules.Settings{
		DefaultChecks: req.DefaultChecks,
		DefaultStyles: req.DefaultStyles,
		ReviewBias:    req.ReviewBias,
		RewriteBias:   req.RewriteBias,
	})
	if err := s.rules.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "規則設定保存失敗"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "規則設定已更新"})
}
