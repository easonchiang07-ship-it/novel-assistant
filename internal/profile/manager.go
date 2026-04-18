package profile

import (
	"os"
	"path/filepath"
	"strings"
)

type Manager struct {
	dataDir    string
	Characters []*Character
	Worlds     []*WorldSetting
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) Load() error {
	m.Characters = nil
	m.Worlds = nil

	charDir := filepath.Join(m.dataDir, "characters")
	if entries, err := os.ReadDir(charDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				path := filepath.Join(charDir, e.Name())
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				char := parseCharacter(string(content))
				char.FilePath = path
				m.Characters = append(m.Characters, char)
			}
		}
	}

	worldDir := filepath.Join(m.dataDir, "worldbuilding")
	if entries, err := os.ReadDir(worldDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				path := filepath.Join(worldDir, e.Name())
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				m.Worlds = append(m.Worlds, &WorldSetting{
					Name:       strings.TrimSuffix(e.Name(), ".md"),
					RawContent: string(content),
					FilePath:   path,
				})
			}
		}
	}
	return nil
}

func (m *Manager) FindByName(name string) *Character {
	for _, c := range m.Characters {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (m *Manager) AllNames() []string {
	names := make([]string, len(m.Characters))
	for i, c := range m.Characters {
		names[i] = c.Name
	}
	return names
}

func parseCharacter(content string) *Character {
	char := &Character{RawContent: content}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# 角色："):
			char.Name = strings.TrimPrefix(line, "# 角色：")
		case strings.HasPrefix(line, "# "):
			char.Name = strings.TrimPrefix(line, "# ")
		case strings.HasPrefix(line, "- 個性："):
			char.Personality = strings.TrimPrefix(line, "- 個性：")
		case strings.HasPrefix(line, "- 核心恐懼："):
			char.CoreFear = strings.TrimPrefix(line, "- 核心恐懼：")
		case strings.HasPrefix(line, "- 行為模式："):
			char.Behavior = strings.TrimPrefix(line, "- 行為模式：")
		case strings.HasPrefix(line, "- 弱點："):
			char.Weakness = strings.TrimPrefix(line, "- 弱點：")
		case strings.HasPrefix(line, "- 成長限制："):
			char.GrowthLimit = strings.TrimPrefix(line, "- 成長限制：")
		case strings.HasPrefix(line, "- 說話風格："):
			char.SpeechStyle = strings.TrimPrefix(line, "- 說話風格：")
		}
	}
	return char
}
