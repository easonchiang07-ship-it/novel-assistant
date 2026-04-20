package checker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"novel-assistant/internal/profile"
	"strings"
)

var ErrStyleParseFailure = errors.New("style analysis parse failure")

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

type EmotionPoint struct {
	Segment string  `json:"segment"`
	Score   float64 `json:"score"`
	Label   string  `json:"label"`
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

func (c *Checker) CheckWorldConflictStream(ctx context.Context, worldProfile, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【世界觀與規則設定】
%s

【待審章節】
%s

請依序分析：
1. **世界觀一致性**：章節是否違反既有世界規則、時間線、地點設定、能力限制或勢力關係？（符合 / 不符合）
2. **具體問題**：若有衝突，列出衝突段落並說明與哪條設定矛盾
3. **修正建議**：提供最小修改方案，盡量保留原場景張力
`, worldProfile, chapter)
	return c.stream(ctx, "你是專責小說世界觀審核的編輯，擅長抓出規則、時間線、地點與能力設定的矛盾。請用繁體中文回答。", prompt, w)
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

func (c *Checker) RewriteChapterStream(ctx context.Context, prompt string, w io.Writer) error {
	return c.stream(ctx, "你是專業小說編輯，專責修稿。請直接輸出修訂後的章節內容，必要時可在最前面補一小段修訂說明。請用繁體中文回答。", prompt, w)
}

func (c *Checker) EnhanceSensoryStream(ctx context.Context, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【待強化章節】
%s

請針對以下面向強化此章節的感官描寫：
1. **視覺**：光線、色彩、空間感、動態細節
2. **聽覺**：環境音、聲音質感、靜默對比
3. **嗅覺**：場景氣味、記憶聯想
4. **觸覺**：材質、溫度、身體感受
5. **味覺**（如適用）

規則：不改變情節與對白；感官描寫須符合場景氛圍；直接輸出修訂後的完整章節內容。
`, chapter)
	return c.stream(ctx, "你是專業文學編輯，擅長將平淡場景改寫為富有感官層次的文學散文。請用繁體中文回答。", prompt, w)
}

func (c *Checker) DiagnoseOpeningStream(ctx context.Context, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【待診斷章節】
%s

請模擬普通網路讀者（非文學評論家）閱讀此章節的體驗，從以下五個維度評分並給出建議：

1. **懸念鉤子（Hook）**（1-10分）：開頭是否有讓讀者想繼續讀的疑問或張力？
2. **主角魅力**（1-10分）：主角是否在短時間內展現出鮮明特質或讓人代入的處境？
3. **世界觀吸引力**（1-10分）：設定是否讓讀者感到新奇或有探索欲？
4. **節奏感**（1-10分）：前幾段的閱讀節奏是否流暢、不拖沓？
5. **留存意願**（1-10分）：讀完此章後，讀者會想看下一章嗎？

輸出格式：每項給出分數＋一到兩句說明；最後給出「總體建議」2-3條可立即執行的修改方向。
`, chapter)
	return c.stream(ctx, "你是資深網文平台編輯，擅長從普通讀者視角評估小說開頭的吸引力。請用繁體中文回答。", prompt, w)
}

func (c *Checker) AnalyzeEmotionCurve(ctx context.Context, chapter string) ([]EmotionPoint, error) {
	prompt := fmt.Sprintf(`
【待分析章節】
%s

請將此章節切成 5-10 個語意段落，對每段分析情緒基調。
輸出嚴格 JSON 陣列，不要有任何額外說明文字：
[
  {"segment": "段落摘要（15字以內）", "score": 0.8, "label": "喜悅"},
  {"segment": "...", "score": -0.5, "label": "緊張"}
]
score 規則：-1.0=極度悲傷/恐懼，0=中性，1.0=極度喜悅/興奮
label 從以下選一個：喜悅、緊張、悲傷、憤怒、平靜、恐懼、期待、失落
`, chapter)

	var buf strings.Builder
	if err := c.stream(ctx, "你是情緒分析專家，只輸出 JSON，不輸出任何額外文字。", prompt, &buf); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(buf.String())
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("無法解析情緒曲線回應")
	}
	var points []EmotionPoint
	if err := json.Unmarshal([]byte(raw[start:end+1]), &points); err != nil {
		return nil, fmt.Errorf("JSON 解析失敗：%w", err)
	}
	return points, nil
}

func (c *Checker) AnalyzeStyle(ctx context.Context, text string) (*profile.StyleAnalysis, error) {
	prompt := fmt.Sprintf(`
分析以下文字的寫作風格，以 JSON 格式回傳風格特徵。
欄位：dialogue_ratio（對話比例：低/中/高）、sensory_freq（感官描寫頻率：低/中/高）、
avg_sentence_len（句子長度：短促/中等/綿長）、tone（語氣：冷靜/熱血/詩意/幽默）、
summary（一句話描述此風格）。
只回傳 JSON，不要其他說明。

文字內容：
%s
`, text)

	var buf strings.Builder
	if err := c.stream(ctx, "你是小說風格分析器，只能輸出合法 JSON，不要輸出任何額外說明。", prompt, &buf); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(buf.String())
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("%w: 無法解析風格分析回應", ErrStyleParseFailure)
	}

	var analysis profile.StyleAnalysis
	if err := json.Unmarshal([]byte(raw[start:end+1]), &analysis); err != nil {
		return nil, fmt.Errorf("%w: JSON 解析失敗：%v", ErrStyleParseFailure, err)
	}
	return &analysis, nil
}

func (c *Checker) ChatWithCharacterStream(ctx context.Context, characterProfile, history, userMessage string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【對話歷史】
%s

【使用者說】
%s

請以角色身份回應，保持角色設定的語氣、用詞習慣與思考邏輯。不要跳出角色，不要說明自己是 AI。
回應長度：2-6句話，自然對話節奏。
`, history, userMessage)

	system := fmt.Sprintf(`你正在扮演以下角色，嚴格依照設定回應：

%s

規則：只說角色會說的話；保持設定中的語氣與個性；不要使用角色沒有的知識；請用繁體中文回答。`, characterProfile)

	return c.stream(ctx, system, prompt, w)
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
