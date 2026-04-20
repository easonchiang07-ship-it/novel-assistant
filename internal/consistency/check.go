package consistency

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Conflict struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Reference   string `json:"reference"`
}

type Checker struct {
	baseURL string
	model   string
}

func New(baseURL, model string) *Checker {
	return &Checker{baseURL: baseURL, model: model}
}

type genReq struct {
	Model  string `json:"model"`
	System string `json:"system"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type genChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (c *Checker) Check(ctx context.Context, prompt, worldContext string) ([]Conflict, error) {
	reqBody := genReq{
		Model:  c.model,
		System: "你是小說邏輯審查員，只輸出 JSON，不輸出任何額外文字。請用繁體中文撰寫衝突說明。",
		Prompt: fmt.Sprintf(`
你是小說邏輯審查員。以下是目前世界設定與追蹤器：

【世界設定與追蹤器】
%s

【使用者即將生成的場景描述】
%s

請判斷場景描述是否與世界設定產生邏輯矛盾。
只回傳 JSON 陣列，格式：
[{"severity":"warning|error","description":"衝突說明","reference":"依據來源"}]
若無衝突，回傳：[]
`, strings.TrimSpace(worldContext), strings.TrimSpace(prompt)),
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal consistency request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollama generate failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var raw strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var chunk genChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		raw.WriteString(chunk.Response)
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	text := strings.TrimSpace(raw.String())
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("無法解析一致性預檢回應")
	}
	var conflicts []Conflict
	if err := json.Unmarshal([]byte(text[start:end+1]), &conflicts); err != nil {
		return nil, fmt.Errorf("JSON 解析失敗：%w", err)
	}
	return conflicts, nil
}
