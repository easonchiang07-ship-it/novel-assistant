package setup

// ModelRole distinguishes language models from embedding models.
type ModelRole string

const (
	RoleLLM   ModelRole = "llm"
	RoleEmbed ModelRole = "embed"
)

// ModelTier is a rough quality/size tier for display purposes.
type ModelTier string

const (
	TierUltraLight  ModelTier = "ultra_light"
	TierLight       ModelTier = "light"
	TierBalanced    ModelTier = "balanced"
	TierHighQuality ModelTier = "high_quality"
)

// ModelOption describes an available Ollama model with its requirements.
type ModelOption struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	SizeGB      float64   `json:"size_gb"`
	MinRAMGB    float64   `json:"min_ram_gb"`
	Role        ModelRole `json:"role"`
	Tier        ModelTier `json:"tier"`
	Description string    `json:"description"`
}

// LLMModels is the ordered list of supported language models.
var LLMModels = []ModelOption{
	{
		Name:        "llama3.2:1b",
		DisplayName: "LLaMA 3.2 1B",
		SizeGB:      1.3,
		MinRAMGB:    4,
		Role:        RoleLLM,
		Tier:        TierUltraLight,
		Description: "超輕量，速度最快，適合 4 GB 記憶體，寫作品質基本",
	},
	{
		Name:        "phi3:mini",
		DisplayName: "Phi-3 Mini",
		SizeGB:      2.3,
		MinRAMGB:    4,
		Role:        RoleLLM,
		Tier:        TierLight,
		Description: "Microsoft 輕量模型，4 GB 可用，品質比 1B 好",
	},
	{
		Name:        "llama3.2",
		DisplayName: "LLaMA 3.2 3B",
		SizeGB:      2.0,
		MinRAMGB:    8,
		Role:        RoleLLM,
		Tier:        TierBalanced,
		Description: "均衡推薦，速度與品質兼顧，需 8 GB",
	},
	{
		Name:        "llama3.1:8b",
		DisplayName: "LLaMA 3.1 8B",
		SizeGB:      4.7,
		MinRAMGB:    16,
		Role:        RoleLLM,
		Tier:        TierHighQuality,
		Description: "高品質寫作分析，需 16 GB，效果最佳",
	},
	{
		Name:        "mistral",
		DisplayName: "Mistral 7B",
		SizeGB:      4.1,
		MinRAMGB:    16,
		Role:        RoleLLM,
		Tier:        TierHighQuality,
		Description: "高品質替代方案，支援多語言，需 16 GB",
	},
}

// EmbedModels is the list of supported embedding models (currently only one).
var EmbedModels = []ModelOption{
	{
		Name:        "nomic-embed-text",
		DisplayName: "Nomic Embed Text",
		SizeGB:      0.27,
		MinRAMGB:    2,
		Role:        RoleEmbed,
		Tier:        TierBalanced,
		Description: "向量搜尋核心元件，體積小，必須安裝",
	},
}

// Recommendation holds the suggested models for a given machine.
type Recommendation struct {
	LLMModelName   string `json:"llm_model_name"`
	EmbedModelName string `json:"embed_model_name"`
	Note           string `json:"note"`
}

// Recommend picks the best LLM for the detected specs.
func Recommend(specs SystemSpecs) Recommendation {
	// Use GPU VRAM as effective memory when a dedicated GPU is present.
	effective := specs.RAMGB
	if specs.VRAMGB >= 8 {
		effective = specs.VRAMGB
	}

	var llm string
	var note string
	switch {
	case effective >= 16:
		llm = "llama3.1:8b"
		note = "你的規格優秀，推薦高品質模型以獲得最佳寫作分析效果"
	case effective >= 8:
		llm = "llama3.2"
		note = "推薦均衡模型，速度與品質兼顧"
	case effective >= 4:
		llm = "llama3.2:1b"
		note = "記憶體較少，推薦輕量模型，仍可正常使用所有功能"
	default:
		llm = "llama3.2:1b"
		note = "記憶體低於 4 GB，建議關閉其他應用程式後再使用"
	}

	return Recommendation{
		LLMModelName:   llm,
		EmbedModelName: "nomic-embed-text",
		Note:           note,
	}
}
