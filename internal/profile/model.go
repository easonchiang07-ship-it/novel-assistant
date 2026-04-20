package profile

type Character struct {
	Name        string                `json:"name"`
	Personality string                `json:"personality"`
	CoreFear    string                `json:"core_fear"`
	Behavior    string                `json:"behavior"`
	Weakness    string                `json:"weakness"`
	GrowthLimit string                `json:"growth_limit"`
	SpeechStyle string                `json:"speech_style"`
	Appearances []CharacterAppearance `json:"appearances"`
	RawContent  string                `json:"raw_content"`
	FilePath    string                `json:"file_path"`
}

type CharacterAppearance struct {
	ChapterTitle string `json:"chapter_title"`
	FileName     string `json:"file_name"`
}

type WorldSetting struct {
	Name       string `json:"name"`
	RawContent string `json:"raw_content"`
	FilePath   string `json:"file_path"`
}

type StyleAnalysis struct {
	DialogueRatio  string `json:"dialogue_ratio"`
	SensoryFreq    string `json:"sensory_freq"`
	AvgSentenceLen string `json:"avg_sentence_len"`
	Tone           string `json:"tone"`
	Summary        string `json:"summary"`
}

type StyleGuide struct {
	Name          string         `json:"name"`
	Perspective   string         `json:"perspective"`    // 敘事視角
	SentenceStyle string         `json:"sentence_style"` // 句式風格
	Rhythm        string         `json:"rhythm"`         // 節奏感
	Tone          string         `json:"tone"`           // 語氣
	Forbidden     string         `json:"forbidden"`      // 禁忌
	Analysis      *StyleAnalysis `json:"analysis,omitempty"`
	RawContent    string         `json:"raw_content"`
	FilePath      string         `json:"file_path"`
}
