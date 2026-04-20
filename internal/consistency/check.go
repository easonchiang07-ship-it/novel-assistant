package consistency

import (
	"context"
	"encoding/json"
	"fmt"
	"novel-assistant/internal/checker"
	"strings"
)

type Conflict struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Reference   string `json:"reference"`
}

type Checker struct {
	checker *checker.Checker
}

func New(c *checker.Checker) *Checker {
	return &Checker{checker: c}
}

func (c *Checker) Check(ctx context.Context, prompt, worldContext string) ([]Conflict, error) {
	if c == nil || c.checker == nil {
		return nil, fmt.Errorf("一致性檢查器尚未初始化")
	}
	var raw strings.Builder
	if err := c.checker.RawStream(
		ctx,
		"你是小說邏輯審查員，只輸出 JSON，不輸出任何額外文字。請用繁體中文撰寫衝突說明。",
		fmt.Sprintf(`
【世界設定與追蹤器】
%s

【使用者即將生成的場景描述】
%s

請判斷場景描述是否與世界設定產生邏輯矛盾。
只回傳 JSON 陣列，格式：
[{"severity":"warning|error","description":"衝突說明","reference":"依據來源"}]
若無衝突，回傳：[]
`, strings.TrimSpace(worldContext), strings.TrimSpace(prompt)),
		&raw,
	); err != nil {
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
