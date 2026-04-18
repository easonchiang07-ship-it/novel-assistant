package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"novel-assistant/internal/extractor"

	"github.com/gin-gonic/gin"
)

type chapterOverview struct {
	Name            string
	Title           string
	WordCount       int
	SceneCount      int
	UpdatedAt       time.Time
	Characters      []string
	ReviewCount     int
	OpenForeshadows int
	TimelineCount   int
	Signals         extractor.Signals
	SceneCards      []sceneBoardCard
}

type sceneBoardCard struct {
	Position     int
	Index        int
	Title        string
	Preview      string
	Synopsis     string
	POV          string
	Conflict     string
	Purpose      string
	Status       string
	ReviewCount  int
	RewriteCount int
}

type candidateCreateRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func chapterNumberFromName(name string) int {
	title := chapterTitle(name)
	digits := make([]rune, 0, len(title))
	for _, r := range title {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		} else if len(digits) > 0 {
			break
		}
	}
	if len(digits) == 0 {
		return 0
	}
	var value int
	fmt.Sscanf(string(digits), "%d", &value)
	return value
}

func (s *Server) buildChapterOverviews() ([]chapterOverview, error) {
	files, err := s.listChapterFiles()
	if err != nil {
		return nil, err
	}

	historyCounts := make(map[string]int)
	sceneReviews := make(map[string]int)
	sceneRewrites := make(map[string]int)
	for _, entry := range s.history.Recent(0) {
		key := entry.ChapterFile
		if key == "" && entry.ChapterTitle != "" {
			key = entry.ChapterTitle + ".md"
		}
		if key != "" {
			historyCounts[key]++
			if scene := strings.TrimSpace(entry.SceneTitle); scene != "" {
				sceneKey := key + "::" + scene
				if entry.Kind == "rewrite" {
					sceneRewrites[sceneKey]++
				} else {
					sceneReviews[sceneKey]++
				}
			}
		}
	}

	timelineCounts := make(map[int]int)
	for _, item := range s.timeline.GetSorted() {
		timelineCounts[item.Chapter]++
	}

	foreshadowCounts := make(map[int]int)
	for _, item := range s.foreshadow.GetAll() {
		if item.Status == "未回收" {
			foreshadowCounts[item.Chapter]++
		}
	}

	overviews := make([]chapterOverview, 0, len(files))
	for _, file := range files {
		path := filepath.Join(s.chapterDir(), file.Name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		text := string(content)
		signals := extractor.AnalyzeChapter(text, s.profiles.AllNames())
		chapterNo := chapterNumberFromName(file.Name)
		scenes := parseScenes(text)
		scenePlans, err := s.loadScenePlans(file.Name)
		if err != nil {
			log.Printf("scene plans load %s: %v", file.Name, err)
			scenePlans = map[string]scenePlan{}
		}
		sceneOrder, err := s.loadScenePlanOrder(file.Name)
		if err != nil {
			log.Printf("scene plan order load %s: %v", file.Name, err)
		}
		sceneCards := make([]sceneBoardCard, 0, len(scenes))
		for idx, scene := range orderedScenes(scenes, sceneOrder) {
			plan := scenePlans[scene.Title]
			sceneKey := file.Name + "::" + scene.Title
			reviewCount := sceneReviews[sceneKey]
			rewriteCount := sceneRewrites[sceneKey]
			status := "draft"
			if rewriteCount > 0 {
				status = "rewritten"
			} else if reviewCount > 0 {
				status = "reviewed"
			}
			sceneCards = append(sceneCards, sceneBoardCard{
				Position:     idx + 1,
				Index:        scene.Index,
				Title:        scene.Title,
				Preview:      scenePreview(scene.Content),
				Synopsis:     plan.Synopsis,
				POV:          plan.POV,
				Conflict:     plan.Conflict,
				Purpose:      plan.Purpose,
				Status:       status,
				ReviewCount:  reviewCount,
				RewriteCount: rewriteCount,
			})
		}
		overviews = append(overviews, chapterOverview{
			Name:            file.Name,
			Title:           file.Title,
			WordCount:       len([]rune(strings.TrimSpace(text))),
			SceneCount:      len(scenes),
			UpdatedAt:       info.ModTime(),
			Characters:      signals.KnownCharacters,
			ReviewCount:     historyCounts[file.Name],
			OpenForeshadows: foreshadowCounts[chapterNo],
			TimelineCount:   timelineCounts[chapterNo],
			Signals:         signals,
			SceneCards:      sceneCards,
		})
	}

	sort.Slice(overviews, func(i, j int) bool {
		return overviews[i].Name < overviews[j].Name
	})
	return overviews, nil
}

func scenePreview(content string) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if text == "" {
		return "尚未填寫場景內容。"
	}
	const limit = 120
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func draftTargetPath(dataDir, kind, name string) (string, string, error) {
	safeName, err := normalizeChapterName(name)
	if err != nil {
		return "", "", err
	}
	baseName := strings.TrimSuffix(safeName, ".md") + ".md"
	switch kind {
	case "character":
		return filepath.Join(dataDir, "characters", baseName), baseName, nil
	case "world":
		return filepath.Join(dataDir, "worldbuilding", baseName), baseName, nil
	default:
		return "", "", fmt.Errorf("不支援的候選類型：%s", kind)
	}
}

func candidateDraftContent(kind, name string) string {
	switch kind {
	case "character":
		return fmt.Sprintf("# 角色：%s\n- 個性：待補充\n- 核心恐懼：待補充\n- 行為模式：待補充\n- 弱點：待補充\n- 成長限制：待補充\n- 說話風格：待補充\n", name)
	default:
		return fmt.Sprintf("# %s\n\n- 地點 / 設定：待補充\n- 規則：待補充\n- 與主線的關聯：待補充\n", name)
	}
}

func (s *Server) handleChaptersPage(c *gin.Context) {
	overviews, err := s.buildChapterOverviews()
	if err != nil {
		c.String(http.StatusInternalServerError, "讀取章節總覽失敗：%s", err.Error())
		return
	}
	c.HTML(http.StatusOK, "chapters.html", gin.H{
		"Title":    "章節總覽",
		"Chapters": overviews,
	})
}

func (s *Server) handleAnalyzeChapter(c *gin.Context) {
	file, err := s.loadChapterFile(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	signals := extractor.AnalyzeChapter(file.Content, s.profiles.AllNames())
	c.JSON(http.StatusOK, gin.H{"item": signals})
}

func (s *Server) handleSaveScenePlan(c *gin.Context) {
	chapterName := c.Param("name")
	file, err := s.loadChapterFile(chapterName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req scenePlanRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := strings.TrimSpace(req.SceneTitle)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "場景標題不可為空"})
		return
	}
	if sceneByTitle(file.Scenes, title) == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "找不到指定場景"})
		return
	}

	if err := s.saveScenePlan(file.Name, scenePlan{
		Title:    title,
		Synopsis: req.Synopsis,
		POV:      req.POV,
		Conflict: req.Conflict,
		Purpose:  req.Purpose,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已儲存場景規劃"})
}

func (s *Server) handleCreateCandidateDraft(c *gin.Context) {
	var req candidateCreateRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path, fileName, err := draftTargetPath(s.cfg.DataDir, strings.TrimSpace(req.Type), strings.TrimSpace(req.Name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := os.Stat(path); err == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "草稿檔已存在", "file": fileName})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(candidateDraftContent(req.Type, req.Name)), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已建立設定草稿，記得重新索引", "file": fileName})
}
