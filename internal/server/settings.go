package server

import (
	"net/http"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewrules"

	"github.com/gin-gonic/gin"
)

type settingsSaveRequest struct {
	DefaultChecks []string `json:"default_checks"`
	DefaultStyles []string `json:"default_styles"`
	ReviewBias    string   `json:"review_bias"`
	RewriteBias   string   `json:"rewrite_bias"`
	OllamaURL     string   `json:"ollama_url"`
	LLMModel      string   `json:"llm_model"`
	EmbedModel    string   `json:"embed_model"`
	Port          string   `json:"port"`
}

func (s *Server) handleSettingsPage(c *gin.Context) {
	settings := s.rules.Get()
	project := s.project.Get()
	backups, _ := listBackupItems(s.backupDir())
	c.HTML(http.StatusOK, "settings.html", gin.H{
		"Title":         "規則設定",
		"Settings":      settings,
		"Project":       project,
		"Backups":       backups,
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

	s.project.Update(projectsettings.Settings{
		OllamaURL:  req.OllamaURL,
		LLMModel:   req.LLMModel,
		EmbedModel: req.EmbedModel,
		Port:       req.Port,
		DataDir:    s.cfg.DataDir,
	})
	if err := s.project.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "專案設定保存失敗"})
		return
	}
	s.applyProjectSettings()
	s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)
	s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "規則與專案設定已更新；若你修改了 Port，需重啟服務後才會生效"})
}
