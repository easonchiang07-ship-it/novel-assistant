package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type scenePlan struct {
	Title    string `json:"title"`
	Synopsis string `json:"synopsis,omitempty"`
	POV      string `json:"pov,omitempty"`
	Conflict string `json:"conflict,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
}

type scenePlanFile struct {
	Items []scenePlan `json:"items"`
}

type scenePlanRequest struct {
	SceneTitle string `json:"scene_title"`
	Synopsis   string `json:"synopsis"`
	POV        string `json:"pov"`
	Conflict   string `json:"conflict"`
	Purpose    string `json:"purpose"`
}

func (s *Server) scenePlanPath(chapterName string) (string, error) {
	normalized, err := normalizeChapterName(chapterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.chapterDir(), normalized+".scenes.json"), nil
}

func (s *Server) loadScenePlans(chapterName string) (map[string]scenePlan, error) {
	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]scenePlan{}, nil
	}
	if err != nil {
		return nil, err
	}

	var stored scenePlanFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	items := make(map[string]scenePlan, len(stored.Items))
	for _, item := range stored.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		item.Title = title
		items[title] = item
	}
	return items, nil
}

func (s *Server) saveScenePlan(chapterName string, plan scenePlan) error {
	plan.Title = strings.TrimSpace(plan.Title)
	plan.Synopsis = strings.TrimSpace(plan.Synopsis)
	plan.POV = strings.TrimSpace(plan.POV)
	plan.Conflict = strings.TrimSpace(plan.Conflict)
	plan.Purpose = strings.TrimSpace(plan.Purpose)
	if plan.Title == "" {
		return fmt.Errorf("場景標題不可為空")
	}

	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return err
	}

	items, err := s.loadScenePlans(chapterName)
	if err != nil {
		return err
	}
	items[plan.Title] = plan

	out := scenePlanFile{Items: make([]scenePlan, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, item)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
