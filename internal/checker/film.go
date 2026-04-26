package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FilmScene represents one cinematic scene extracted from a chapter.
type FilmScene struct {
	SceneID     string          `yaml:"scene_id"     json:"scene_id"`
	Location    string          `yaml:"location"     json:"location"`
	Time        string          `yaml:"time"         json:"time"`
	Mood        string          `yaml:"mood"         json:"mood"`
	Characters  []FilmCharacter `yaml:"characters"   json:"characters"`
	Dialogue    []FilmLine      `yaml:"dialogue"     json:"dialogue"`
	VideoPrompt string          `yaml:"video_prompt" json:"video_prompt"`
}

// FilmCharacter describes one character's physical presence in a scene.
type FilmCharacter struct {
	Name       string `yaml:"name"                 json:"name"`
	Appearance string `yaml:"appearance,omitempty" json:"appearance,omitempty"`
	Action     string `yaml:"action"               json:"action"`
	Emotion    string `yaml:"emotion"              json:"emotion"`
}

// FilmLine is one line of dialogue or narrated speech in a scene.
type FilmLine struct {
	Speaker string `yaml:"speaker" json:"speaker"`
	Text    string `yaml:"text"    json:"text"`
}

// ExtractFilmScenes parses a chapter into a list of cinematic scenes.
// Psychological descriptions are translated into visible physical actions.
// The video_prompt field is written in English for direct use with video AI tools.
func (c *Checker) ExtractFilmScenes(ctx context.Context, chapterFile, chapter string) ([]FilmScene, error) {
	prompt := fmt.Sprintf(`分析以下小說章節，拆解成一個或多個電影場景。

關鍵轉譯規則：
- 「心理描寫」必須轉成「可視覺化動作」。例如：
  「他感到後悔」→ action: "低下頭，手指不斷摩擦杯緣"
  「她很憤怒」→ action: "緊握拳頭，轉身背對對方"
  「他感到孤獨」→ action: "獨自坐在空蕩的車站，手裡握著一張揉皺的車票"
- video_prompt 必須用英文，描述導演可以實際拍攝的畫面，包含光線、氣氛、鏡頭感
- location 用中文描述具體地點
- time 用中文（例：夜晚、清晨、下午）
- mood 用中文（例：緊繃、哀傷、溫暖）
- 每個 scene_id 格式：%s-S01、%s-S02 依此類推

輸出嚴格 JSON 陣列，不加任何說明：
[
  {
    "scene_id": "...",
    "location": "...",
    "time": "...",
    "mood": "...",
    "characters": [
      {"name": "...", "appearance": "...", "action": "...", "emotion": "..."}
    ],
    "dialogue": [
      {"speaker": "...", "text": "..."}
    ],
    "video_prompt": "..."
  }
]

【章節內容】
%s`, chapterFile, chapterFile, chapter)

	var buf strings.Builder
	if err := c.llm.Stream(ctx,
		"你是專業電影前製導演，擅長把小說場景轉換成可拍攝的視覺化腳本。只輸出 JSON，不輸出任何說明。",
		prompt, &buf); err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(buf.String())
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("無法解析 film scenes 回應")
	}

	var scenes []FilmScene
	if err := json.Unmarshal([]byte(raw[start:end+1]), &scenes); err != nil {
		return nil, fmt.Errorf("JSON 解析失敗：%w", err)
	}
	return scenes, nil
}
