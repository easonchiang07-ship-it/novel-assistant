package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Order []string    `json:"order,omitempty"`
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
	s.scenePlansMu.RLock()
	defer s.scenePlansMu.RUnlock()

	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return nil, err
	}
	return loadScenePlansFromPath(path)
}

func (s *Server) loadScenePlanOrder(chapterName string) ([]string, error) {
	s.scenePlansMu.RLock()
	defer s.scenePlansMu.RUnlock()

	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return nil, err
	}
	stored, err := loadScenePlanFile(path)
	if err != nil {
		return nil, err
	}
	return sanitizeSceneOrder(stored.Order), nil
}

func loadScenePlansFromPath(path string) (map[string]scenePlan, error) {
	stored, err := loadScenePlanFile(path)
	if err != nil {
		return nil, err
	}

	items := make(map[string]scenePlan, len(stored.Items))
	for _, item := range stored.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		// Plans are keyed by scene title, so renaming a scene in markdown can
		// leave behind orphaned sidecar entries until a future cleanup step prunes them.
		item.Title = title
		items[title] = item
	}
	return items, nil
}

func loadScenePlanFile(path string) (scenePlanFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return scenePlanFile{}, nil
	}
	if err != nil {
		return scenePlanFile{}, err
	}

	var stored scenePlanFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return scenePlanFile{}, err
	}
	return stored, nil
}

func sanitizeSceneOrder(order []string) []string {
	out := make([]string, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, title := range order {
		clean := strings.TrimSpace(title)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
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

	s.scenePlansMu.Lock()
	defer s.scenePlansMu.Unlock()

	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return err
	}

	stored, err := loadScenePlanFile(path)
	if err != nil {
		return err
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
	items[plan.Title] = plan

	out := scenePlanFile{
		Order: sanitizeSceneOrder(stored.Order),
		Items: make([]scenePlan, 0, len(items)),
	}
	for _, item := range items {
		out.Items = append(out.Items, item)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		return out.Items[i].Title < out.Items[j].Title
	})

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return writeFileReplace(path, data, 0644)
}

func (s *Server) saveScenePlanOrder(chapterName string, order []string) error {
	s.scenePlansMu.Lock()
	defer s.scenePlansMu.Unlock()

	path, err := s.scenePlanPath(chapterName)
	if err != nil {
		return err
	}
	stored, err := loadScenePlanFile(path)
	if err != nil {
		return err
	}
	stored.Order = sanitizeSceneOrder(order)
	sort.Slice(stored.Items, func(i, j int) bool {
		return stored.Items[i].Title < stored.Items[j].Title
	})
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return writeFileReplace(path, data, 0644)
}

func writeFileReplace(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}

	// Write to a temp file first so the existing sidecar is never replaced with
	// a partially written file if the process dies mid-write.
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}

	// Windows does not replace existing files on rename, so fall back to a
	// best-effort replace after the temp file is fully written.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, path)
}
