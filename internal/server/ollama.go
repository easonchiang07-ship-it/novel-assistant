package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
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

// inflight tracks models currently being pulled; value is a cancel func.
var (
	inflightMu sync.Mutex
	inflight   = map[string]context.CancelFunc{}
)

func (s *Server) handleOllamaPull(c *gin.Context) {
	var req struct {
		Model string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請求格式錯誤"})
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model 不可為空"})
		return
	}

	inflightMu.Lock()
	if _, ok := inflight[model]; ok {
		inflightMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("模型 %s 已在下載中", model)})
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	inflight[model] = cancel
	inflightMu.Unlock()

	defer func() {
		inflightMu.Lock()
		delete(inflight, model)
		inflightMu.Unlock()
		cancel()
	}()

	body, _ := json.Marshal(map[string]any{"model": model, "stream": true})
	pullReq, err := http.NewRequestWithContext(ctx, "POST", s.cfg.OllamaURL+"/api/pull",
		strings.NewReader(string(body)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pullReq.Header.Set("Content-Type", "application/json")

	// Dial timeout prevents hanging on unreachable hosts; response streaming follows client context.
	client := &http.Client{Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: 5 * time.Second,
	}}
	pullResp, err := client.Do(pullReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "無法連線 Ollama：" + err.Error()})
		return
	}
	defer pullResp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	flusher, _ := c.Writer.(http.Flusher)

	scanner := bufio.NewScanner(pullResp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal(line, &frame); err != nil {
			continue
		}
		if errMsg, ok := frame["error"].(string); ok {
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n",
				mustJSON(map[string]string{"error": errMsg}))
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		if status, _ := frame["status"].(string); status == "success" {
			fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", mustJSON(frame))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n",
			mustJSON(map[string]string{"error": "stream read error: " + err.Error()}))
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
