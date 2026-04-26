package exporter

import (
	"encoding/json"
	"fmt"

	"novel-assistant/internal/checker"

	"gopkg.in/yaml.v3"
)

// FilmFormat is the output format for film scene export.
type FilmFormat string

const (
	FilmFormatYAML FilmFormat = "yaml"
	FilmFormatJSON FilmFormat = "json"
)

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
