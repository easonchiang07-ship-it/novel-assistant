package exporter

import (
	"encoding/json"
	"fmt"
	"strings"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/profile"

	"gopkg.in/yaml.v3"
)

// FilmFormat is the output format for film scene export.
type FilmFormat string

const (
	FilmFormatYAML FilmFormat = "yaml"
	FilmFormatJSON FilmFormat = "json"
)

// InjectVisualGuard fills each FilmCharacter.Appearance from character profiles
// and prepends appearance anchors to the scene's VideoPrompt, ensuring visual
// consistency across independently generated scene prompts.
// Characters without an Appearance in their profile are left unchanged.
func InjectVisualGuard(scenes []checker.FilmScene, characters []*profile.Character) []checker.FilmScene {
	lookup := make(map[string]string, len(characters))
	for _, c := range characters {
		if c.Appearance != "" {
			lookup[c.Name] = c.Appearance
		}
	}
	if len(lookup) == 0 {
		return scenes
	}

	out := make([]checker.FilmScene, len(scenes))
	for i, scene := range scenes {
		chars := make([]checker.FilmCharacter, len(scene.Characters))
		copy(chars, scene.Characters)
		scene.Characters = chars
		for j := range scene.Characters {
			if app, ok := lookup[scene.Characters[j].Name]; ok {
				scene.Characters[j].Appearance = app
			}
		}
		var anchors []string
		for _, ch := range scene.Characters {
			if ch.Appearance != "" {
				anchors = append(anchors, ch.Name+": "+ch.Appearance)
			}
		}
		if len(anchors) > 0 && scene.VideoPrompt != "" {
			scene.VideoPrompt = "[" + strings.Join(anchors, "; ") + "] " + scene.VideoPrompt
		}
		out[i] = scene
	}
	return out
}

// ExportFilmScenes serialises a list of FilmScene values to the requested format.
func ExportFilmScenes(scenes []checker.FilmScene, format FilmFormat) ([]byte, error) {
	switch format {
	case FilmFormatJSON:
		return json.MarshalIndent(scenes, "", "  ")
	default:
		out, err := yaml.Marshal(scenes)
		if err != nil {
			return nil, fmt.Errorf("yaml marshal: %w", err)
		}
		return out, nil
	}
}
