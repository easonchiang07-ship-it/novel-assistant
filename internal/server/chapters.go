package server

import (
	"fmt"
	"net/http"
	"novel-assistant/internal/vectorstore"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Scene represents one scene inside a chapter file.
// Scene markers use the format "## Scene N" or "## Scene N: Title".
// Files without any markers are treated as a single implicit scene (Scenes is nil).
type Scene struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// sceneHeaderRe matches lines of the form "## Scene 1" or "## Scene 2: The Rain".
var sceneHeaderRe = regexp.MustCompile(`(?m)^(## Scene \d+(?::\s*.+)?)$`)
var chapterIndexRe = regexp.MustCompile(`第\s*(\d+)\s*章`)

// parseScenes splits chapter content into scenes when scene markers are present.
// Returns nil when no markers are found (caller treats content as a single implicit scene).
func parseScenes(content string) []Scene {
	locs := sceneHeaderRe.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return nil
	}

	scenes := make([]Scene, 0, len(locs))
	for i, loc := range locs {
		header := content[loc[0]:loc[1]]
		title := strings.TrimPrefix(header, "## ")

		start := loc[1]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		scenes = append(scenes, Scene{
			Index:   i + 1,
			Title:   strings.TrimSpace(title),
			Content: strings.TrimSpace(content[start:end]),
		})
	}
	return scenes
}

func extractChapterIndex(name string) int {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	matches := chapterIndexRe.FindStringSubmatch(base)
	if len(matches) != 2 {
		return 0
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return value
}

func chunkChapter(name string, content string) []vectorstore.Document {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	chapterIndex := extractChapterIndex(name)
	scenes := parseScenes(trimmed)
	if len(scenes) > 0 {
		chunks := make([]vectorstore.Document, 0, len(scenes))
		for _, scene := range scenes {
			chunks = append(chunks, vectorstore.Document{
				ID:           fmt.Sprintf("chapter_%s_scene_%d", name, scene.Index),
				Type:         "chapter",
				Content:      scene.Content,
				ChapterFile:  name,
				ChapterIndex: chapterIndex,
				SceneIndex:   scene.Index,
				ChunkType:    "scene",
			})
		}
		return chunks
	}

	parts := strings.Split(trimmed, "\n\n")
	chunks := make([]vectorstore.Document, 0, len(parts))
	paragraphIndex := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		paragraphIndex++
		chunks = append(chunks, vectorstore.Document{
			ID:           fmt.Sprintf("chapter_%s_para_%d", name, paragraphIndex),
			Type:         "chapter",
			Content:      part,
			ChapterFile:  name,
			ChapterIndex: chapterIndex,
			SceneIndex:   paragraphIndex,
			ChunkType:    "paragraph",
		})
	}
	return chunks
}

// sceneByTitle returns the first scene whose Title matches, or nil.
func sceneByTitle(scenes []Scene, title string) *Scene {
	title = strings.TrimSpace(title)
	for i := range scenes {
		if scenes[i].Title == title {
			return &scenes[i]
		}
	}
	return nil
}

type chapterFile struct {
	Name    string  `json:"name"`
	Title   string  `json:"title"`
	Content string  `json:"content,omitempty"`
	Scenes  []Scene `json:"scenes,omitempty"`
}

type chapterSaveRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func normalizeChapterName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("章節檔名不可為空")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("章節檔名格式不合法")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("章節檔名格式不合法")
	}
	return name, nil
}

func chapterTitle(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func chapterDirFor(dataDir string) string {
	return filepath.Join(dataDir, "chapters")
}

func (s *Server) chapterDir() string {
	return chapterDirFor(s.cfg.DataDir)
}

func (s *Server) listChapterFiles() ([]chapterFile, error) {
	entries, err := os.ReadDir(s.chapterDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	files := make([]chapterFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		files = append(files, chapterFile{
			Name:  entry.Name(),
			Title: chapterTitle(entry.Name()),
		})
	}

	order, err := s.loadChapterOrder()
	if err != nil {
		return nil, err
	}
	return orderedChapterFiles(files, order), nil
}

func (s *Server) loadChapterFile(name string) (chapterFile, error) {
	normalized, err := normalizeChapterName(name)
	if err != nil {
		return chapterFile{}, err
	}

	path := filepath.Join(s.chapterDir(), normalized)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return chapterFile{}, fmt.Errorf("找不到章節檔案：%s", normalized)
		}
		return chapterFile{}, err
	}

	text := string(content)
	return chapterFile{
		Name:    normalized,
		Title:   chapterTitle(normalized),
		Content: text,
		Scenes:  parseScenes(text),
	}, nil
}

func (s *Server) saveChapterFile(name, content string) (chapterFile, error) {
	normalized, err := normalizeChapterName(name)
	if err != nil {
		return chapterFile{}, err
	}
	if err := os.MkdirAll(s.chapterDir(), 0755); err != nil {
		return chapterFile{}, err
	}

	path := filepath.Join(s.chapterDir(), normalized)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return chapterFile{}, err
	}

	return chapterFile{
		Name:    normalized,
		Title:   chapterTitle(normalized),
		Content: content,
		Scenes:  parseScenes(content),
	}, nil
}

func (s *Server) handleListChapters(c *gin.Context) {
	files, err := s.listChapterFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": files})
}

// ChapterContent returns the raw text of a chapter. Used by the desktop binding layer.
func (s *Server) ChapterContent(name string) (string, error) {
	f, err := s.loadChapterFile(name)
	if err != nil {
		return "", err
	}
	return f.Content, nil
}

// SaveChapterContent writes a chapter file and returns the normalised title.
// Used by the desktop binding layer.
func (s *Server) SaveChapterContent(name, content string) (string, error) {
	f, err := s.saveChapterFile(name, content)
	if err != nil {
		return "", err
	}
	return f.Title, nil
}

func (s *Server) handleGetChapter(c *gin.Context) {
	file, err := s.loadChapterFile(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, file)
}

func (s *Server) handleSaveChapter(c *gin.Context) {
	var req chapterSaveRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	file, err := s.saveChapterFile(req.Name, req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, file)
}
