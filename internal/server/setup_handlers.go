package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"novel-assistant/internal/setup"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleSetupSpecs returns detected system specs, model list, and recommendation.
// Returns 403 once setup is complete to avoid exposing hardware info unnecessarily.
func (s *Server) handleSetupSpecs(c *gin.Context) {
	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if setup.IsComplete(dataDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already complete"})
		return
	}
	specs := setup.DetectSpecs()
	rec := setup.Recommend(specs)
	c.JSON(http.StatusOK, gin.H{
		"specs":            specs,
		"recommendation":   rec,
		"llm_models":       setup.LLMModels,
		"embed_models":     setup.EmbedModels,
		"ollama_installed": setup.IsOllamaInstalled(),
	})
}

// handleSetupInstallOllama streams Ollama installation progress via SSE (GET,
// so it is compatible with the browser's EventSource API).
func (s *Server) handleSetupInstallOllama(c *gin.Context) {
	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if setup.IsComplete(dataDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already complete"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	send := func(percent int, msg string) {
		data, _ := json.Marshal(gin.H{"percent": percent, "msg": msg})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data) //nolint:errcheck
		c.Writer.Flush()
	}

	if setup.IsOllamaInstalled() {
		send(100, "Ollama 已安裝")
		return
	}

	if err := setup.InstallOllama(send); err != nil {
		data, _ := json.Marshal(gin.H{"error": err.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data) //nolint:errcheck
		c.Writer.Flush()
	}
}

// handleSetupPullModel is a public GET endpoint that streams an ollama model
// pull via SSE during the setup wizard. It is disabled once setup is complete
// (callers should use the auth-protected /api/ollama/pull instead). The model
// name must be in the known allowlist to prevent arbitrary pulls.
func (s *Server) handleSetupPullModel(c *gin.Context) {
	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if setup.IsComplete(dataDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already complete; use /api/ollama/pull"})
		return
	}

	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model query param is required"})
		return
	}
	if !setup.IsAllowedModel(model) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown model: " + model})
		return
	}
	s.streamOllamaPull(c, model)
}

// handleSetupCheckOllama pings Ollama's /api/tags and returns {"ok": true/false}.
func (s *Server) handleSetupCheckOllama(c *gin.Context) {
	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if setup.IsComplete(dataDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already complete"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.OllamaURL+"/api/tags", nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": fmt.Sprintf("Ollama 回應 %d", resp.StatusCode)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleSetupComplete persists the chosen models and marks setup as done.
func (s *Server) handleSetupComplete(c *gin.Context) {
	dataDir := s.globalDataDir
	if dataDir == "" {
		dataDir = "data"
	}
	// Reject once setup is complete — callers should use the auth-protected
	// settings API to change models after initial setup.
	if setup.IsComplete(dataDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already complete"})
		return
	}

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

	// Validate against the known model allowlist.
	if !setup.IsAllowedModel(req.LLMModel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown llm_model: " + req.LLMModel})
		return
	}
	if !setup.IsAllowedModel(req.EmbedModel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown embed_model: " + req.EmbedModel})
		return
	}

	// Persist the model choices into the project settings store so they
	// survive restart. applyProjectSettings() reads from s.project.Get(),
	// so we must update the store first.
	existing := s.project.Get()
	existing.LLMModel = req.LLMModel
	existing.EmbedModel = req.EmbedModel
	s.project.Update(existing)
	if err := s.project.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存設定失敗：" + err.Error()})
		return
	}
	s.applyProjectSettings()

	if err := setup.MarkComplete(dataDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
