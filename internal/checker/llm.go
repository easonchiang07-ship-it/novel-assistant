package checker

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

// LLMStreamer 是 LLM 呼叫的抽象，寫入 w 並在完成時 return。
type LLMStreamer interface {
	Stream(ctx context.Context, system, prompt string, w io.Writer) error
}

// OllamaStreamer 是現有 Ollama /api/generate 實作。
type OllamaStreamer struct {
	BaseURL string
	Model   string
}

func (o *OllamaStreamer) Stream(ctx context.Context, system, prompt string, w io.Writer) error {
	body, err := json.Marshal(genReq{Model: o.Model, System: system, Prompt: prompt, Stream: true})
	if err != nil {
		return fmt.Errorf("marshal generate request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollama generate failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var chunk genChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if _, err := io.WriteString(w, chunk.Response); err != nil {
			return err
		}
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}
