package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type chapterOrderFile struct {
	Order []string `json:"order"`
}

type orderRequest struct {
	Order []string `json:"order"`
}

func (s *Server) chapterOrderPath() string {
	return filepath.Join(s.cfg.DataDir, "chapter_order.json")
}

func (s *Server) loadChapterOrder() ([]string, error) {
	s.chapterOrderMu.RLock()
	defer s.chapterOrderMu.RUnlock()
	return loadChapterOrderFromPath(s.chapterOrderPath())
}

func loadChapterOrderFromPath(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var stored chapterOrderFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(stored.Order))
	seen := make(map[string]struct{}, len(stored.Order))
	for _, name := range stored.Order {
		normalized, err := normalizeChapterName(name)
		if err != nil {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Server) saveChapterOrder(order []string) error {
	s.chapterOrderMu.Lock()
	defer s.chapterOrderMu.Unlock()

	normalized := make([]string, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, name := range order {
		clean, err := normalizeChapterName(name)
		if err != nil {
			return err
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}

	data, err := json.MarshalIndent(chapterOrderFile{Order: normalized}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.chapterOrderPath()), 0755); err != nil {
		return err
	}
	return writeFileReplace(s.chapterOrderPath(), data, 0644)
}

func orderedChapterFiles(files []chapterFile, order []string) []chapterFile {
	if len(files) == 0 {
		return nil
	}
	if len(order) == 0 {
		sort.Slice(files, func(i, j int) bool {
			return files[i].Name < files[j].Name
		})
		return files
	}

	index := make(map[string]int, len(order))
	for i, name := range order {
		index[name] = i
	}
	sort.SliceStable(files, func(i, j int) bool {
		pi, okI := index[files[i].Name]
		pj, okJ := index[files[j].Name]
		switch {
		case okI && okJ:
			return pi < pj
		case okI:
			return true
		case okJ:
			return false
		default:
			return files[i].Name < files[j].Name
		}
	})
	return files
}

func orderedScenes(scenes []Scene, order []string) []Scene {
	if len(scenes) == 0 || len(order) == 0 {
		return scenes
	}

	copied := make([]Scene, len(scenes))
	copy(copied, scenes)

	index := make(map[string]int, len(order))
	for i, title := range order {
		index[strings.TrimSpace(title)] = i
	}
	sort.SliceStable(copied, func(i, j int) bool {
		pi, okI := index[copied[i].Title]
		pj, okJ := index[copied[j].Title]
		switch {
		case okI && okJ:
			return pi < pj
		case okI:
			return true
		case okJ:
			return false
		default:
			return copied[i].Index < copied[j].Index
		}
	})
	return copied
}

func chapterPreamble(content string) string {
	firstMarker := sceneHeaderRe.FindStringIndex(content)
	if firstMarker == nil || firstMarker[0] <= 0 {
		return ""
	}
	return strings.TrimRight(content[:firstMarker[0]], "\r\n\t ")
}

func rebuildChapterWithSceneOrder(content string, scenes []Scene, order []string) string {
	if len(scenes) == 0 {
		return strings.TrimSpace(content)
	}

	parts := make([]string, 0, len(scenes)+1)
	if preamble := chapterPreamble(content); preamble != "" {
		parts = append(parts, preamble)
	}
	for _, scene := range orderedScenes(scenes, order) {
		parts = append(parts, "## "+scene.Title+"\n"+scene.Content)
	}
	return strings.Join(parts, "\n\n")
}

func (s *Server) handleSaveChapterOrder(c *gin.Context) {
	var req orderRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.saveChapterOrder(req.Order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已儲存章節排序"})
}

func (s *Server) handleSaveSceneOrder(c *gin.Context) {
	chapterName := c.Param("name")
	file, err := s.loadChapterFile(chapterName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req orderRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	allowed := make(map[string]struct{}, len(file.Scenes))
	for _, scene := range file.Scenes {
		allowed[scene.Title] = struct{}{}
	}
	for _, title := range req.Order {
		if _, ok := allowed[strings.TrimSpace(title)]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("找不到場景：%s", title)})
			return
		}
	}
	if err := s.saveScenePlanOrder(file.Name, req.Order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已儲存場景排序"})
}
