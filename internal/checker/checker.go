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

func (c *Checker) CheckBehaviorStream(ctx context.Context, profile, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【角色設定】
%s

【待審章節】
%s

請依序分析：
1. **行為一致性**：角色行為是否符合設定？（符合 / 不符合）
2. **具體問題**：若不符合，指出段落與違和原因
3. **蛻變方案**：若作者希望保留此轉變，請設計合理蛻變過程：
   - 需要哪些觸發事件？
   - 角色內心掙扎為何？
   - 建議幾個章節完成轉變？
`, profile, chapter)
	return c.stream(ctx, "你是嚴謹的小說編輯，專責角色一致性審查。請用繁體中文回答。", prompt, w)
}

func (c *Checker) CheckStyleStream(ctx context.Context, styleProfile, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【寫作風格設定】
%s

【待審章節】
%s

請依序分析：
1. **風格一致性**：章節的寫作風格是否符合設定？（符合 / 不符合）
2. **具體問題**：列出不符合風格設定的段落或句子，說明哪裡違和
3. **修改建議**：針對問題段落，提供符合風格的改寫方向或範例
`, styleProfile, chapter)
	return c.stream(ctx, "你是專業文學編輯，專責分析文章寫作風格的一致性。請用繁體中文回答。", prompt, w)
}

func (c *Checker) CheckDialogueStream(ctx context.Context, name, personality, speechStyle, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【角色說話風格】
姓名：%s
個性：%s
說話風格：%s

【待審章節對白】
%s

請分析：
1. **語氣一致性**：對白是否符合角色風格？
2. **具體問題**：列出不符合風格的對白行
3. **修改建議**：提供符合角色風格的改寫版本
`, name, personality, speechStyle, chapter)
	return c.stream(ctx, "你是專業對白編輯，分析角色說話風格一致性。請用繁體中文回答。", prompt, w)
}

func (c *Checker) stream(ctx context.Context, system, prompt string, w io.Writer) error {
	body, err := json.Marshal(genReq{Model: c.model, System: system, Prompt: prompt, Stream: true})
	if err != nil {
		return fmt.Errorf("marshal generate request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(body))
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

func ExtractNames(text string, knownNames []string) []string {
	var found []string
	for _, name := range knownNames {
		if strings.Contains(text, name) {
			found = append(found, name)
		}
	}
	return found
}
