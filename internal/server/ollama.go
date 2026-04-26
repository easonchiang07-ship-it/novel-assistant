package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var entryAliases = map[string]bool{
	"llama3.2":        true,
	"llama3.2:latest": true,
	"llama3.2:3b":     true,
}

const entryPullTarget = "llama3.2:3b"

func normalizeModel(name string) string {
	return strings.TrimSuffix(name, ":latest")
}

func pullTarget(configured string) string {
	if entryAliases[configured] {
		return entryPullTarget
	}
	return configured
}

func modelReady(configured string, reported []string) bool {
	if entryAliases[configured] {
		for _, r := range reported {
			if entryAliases[r] {
				return true
			}
		}
		return false
	}
	norm := normalizeModel(configured)
	for _, r := range reported {
		if normalizeModel(r) == norm {
			return true
		}
	}
	return false
}

type ollamaTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

type ollamaStatusResponse struct {
	Running        bool     `json:"running"`
	Models         []string `json:"models"`
	LLMReady       bool     `json:"llm_ready"`
	EmbedReady     bool     `json:"embed_ready"`
	LLMModel       string   `json:"llm_model"`
	EmbedModel     string   `json:"embed_model"`
	LLMPullModel   string   `json:"llm_pull_model"`
	EmbedPullModel string   `json:"embed_pull_model"`
}

func (s *Server) handleOllamaStatus(c *gin.Context) {
	llmModel := s.cfg.LLMModel
	embedModel := s.cfg.EmbedModel

	resp := ollamaStatusResponse{
		LLMModel:       llmModel,
		EmbedModel:     embedModel,
		LLMPullModel:   pullTarget(llmModel),
		EmbedPullModel: pullTarget(embedModel),
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.OllamaURL+"/api/tags", nil)
	if err != nil {
		c.JSON(http.StatusOK, resp)
		return
	}

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, resp)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, resp)
		return
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&tags); err != nil {
		c.JSON(http.StatusOK, resp)
		return
	}

	resp.Running = true
	names := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		if name != "" {
			names = append(names, name)
		}
	}
	resp.Models = names
	resp.LLMReady = modelReady(llmModel, names)
	resp.EmbedReady = modelReady(embedModel, names)

	c.JSON(http.StatusOK, resp)
}
