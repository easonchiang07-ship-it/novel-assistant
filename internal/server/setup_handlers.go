package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"novel-assistant/internal/setup"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleSetupSpecs returns detected system specs, model list, and recommendation.
func (s *Server) handleSetupSpecs(c *gin.Context) {
	specs := setup.DetectSpecs()
	rec := setup.Recommend(specs)
	c.JSON(http.StatusOK, gin.H{
		"specs":          specs,
		"recommendation": rec,
		"llm_models":     setup.LLMModels,
		"embed_models":   setup.EmbedModels,
		"ollama_installed": setup.IsOllamaInstalled(),
	})
}

// handleSetupInstallOllama streams Ollama installation progress via SSE.
func (s *Server) handleSetupInstallOllama(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	send := func(percent int, msg string) {
		data, _ := json.Marshal(gin.H{"percent": percent, "msg": msg})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}

	if setup.IsOllamaInstalled() {
		send(100, "Ollama 已安裝")
		return
	}

	if err := setup.InstallOllama(send); err != nil {
		data, _ := json.Marshal(gin.H{"error": err.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}
}

// handleSetupComplete persists the chosen models and marks setup as done.
func (s *Server) handleSetupComplete(c *gin.Context) {
	var req struct {
		LLMModel   string `json:"llm_model"`
		EmbedModel string `json:"embed_model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.LLMModel = strings.TrimSpace(req.LLMModel)
	req.EmbedModel = strings.TrimSpace(req.EmbedModel)
	if req.LLMModel == "" {
		req.LLMModel = "llama3.2"
	}
	if req.EmbedModel == "" {
		req.EmbedModel = "nomic-embed-text"
	}

	// Update running config so the next ingest uses the selected models.
	s.cfg.LLMModel = req.LLMModel
	s.cfg.EmbedModel = req.EmbedModel
	s.applyProjectSettings()

	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if err := setup.MarkComplete(dataDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
