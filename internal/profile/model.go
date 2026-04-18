package profile

type Character struct {
	Name        string `json:"name"`
	Personality string `json:"personality"`
	CoreFear    string `json:"core_fear"`
	Behavior    string `json:"behavior"`
	Weakness    string `json:"weakness"`
	GrowthLimit string `json:"growth_limit"`
	SpeechStyle string `json:"speech_style"`
	RawContent  string `json:"raw_content"`
	FilePath    string `json:"file_path"`
}

type WorldSetting struct {
	Name       string `json:"name"`
	RawContent string `json:"raw_content"`
	FilePath   string `json:"file_path"`
}
