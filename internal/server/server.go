package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"novel-assistant/internal/checker"
	"novel-assistant/internal/config"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"
	"novel-assistant/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type projectState struct {
	dataDir       string
	profiles      *profile.Manager
	store         *vectorstore.Store
	project       *projectsettings.Store
	rules         *reviewrules.Store
	history       *reviewhistory.Store
	relationships *tracker.RelationshipTracker
	timeline      *tracker.TimelineTracker
	foreshadow    *tracker.ForeshadowTracker
}

type Server struct {
	cfg            *config.Config
	router         *gin.Engine
	stateMu        sync.RWMutex
	state          *projectState
	profiles       *profile.Manager
	store          *vectorstore.Store
	project        *projectsettings.Store
	embedder       *embedder.OllamaEmbedder
	checker        *checker.Checker
	rules          *reviewrules.Store
	history        *reviewhistory.Store
	relationships  *tracker.RelationshipTracker
	timeline       *tracker.TimelineTracker
	foreshadow     *tracker.ForeshadowTracker
	chapterOrderMu sync.RWMutex
	scenePlansMu   sync.RWMutex
}

func New(cfg *config.Config) (*Server, error) {
	idx, err := workspace.EnsureIndex()
	if err != nil {
		return nil, fmt.Errorf("load workspace index: %w", err)
	}

	s := &Server{
		cfg:      cfg,
		embedder: embedder.New(cfg.OllamaURL, cfg.EmbedModel),
		checker:  checker.New(cfg.OllamaURL, cfg.LLMModel),
	}

	st, err := s.loadProjectState(idx.Active)
	if err != nil {
		return nil, fmt.Errorf("load project state: %w", err)
	}
	s.setProjectState(st)
	s.applyProjectSettings()

	gin.SetMode(gin.ReleaseMode)
	s.router = gin.Default()
	s.router.SetFuncMap(template.FuncMap{
		"jsonJS": func(v any) template.JS {
			data, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(data)
		},
		"sourceEnabled": func(sources []string, name string) bool {
			for _, source := range sources {
				if source == name {
					return true
				}
			}
			return false
		},
	})
	s.router.LoadHTMLGlob("web/templates/*.html")
	s.router.Static("/static", "web/static")
	s.setupRoutes()

	return s, nil
}

func (s *Server) setupRoutes() {
	r := s.router

	r.GET("/", s.handleIndex)
	r.GET("/chapters", s.handleChaptersPage)
	r.GET("/characters", s.handleCharacters)
	r.GET("/history", s.handleHistoryPage)
	r.GET("/settings", s.handleSettingsPage)
	r.GET("/styles", s.handleStylesPage)
	r.GET("/check", s.handleCheckPage)
	r.GET("/chat", s.handleChatPage)
	r.GET("/relationships", s.handleRelationshipsPage)
	r.GET("/timeline", s.handleTimelinePage)
	r.GET("/foreshadow", s.handleForeshadowPage)
	r.GET("/api/history/:id", s.handleGetHistoryEntry)
	r.GET("/api/history/:id/diff", s.handleGetHistoryDiff)
	r.GET("/api/backups", s.handleListBackups)
	r.GET("/api/projects", s.handleListProjects)
	r.GET("/api/chapters/:name/analysis", s.handleAnalyzeChapter)
	r.GET("/api/chapters", s.handleListChapters)
	r.GET("/api/chapters/:name", s.handleGetChapter)
	r.GET("/api/settings", s.handleGetSettings)

	r.POST("/ingest", s.handleIngest)
	r.POST("/api/chapters", s.handleSaveChapter)
	r.POST("/api/chapters/order", s.handleSaveChapterOrder)
	r.POST("/api/chapters/:name/scenes/plan", s.handleSaveScenePlan)
	r.POST("/api/chapters/:name/scenes/order", s.handleSaveSceneOrder)
	r.POST("/api/backups/create", s.handleCreateBackup)
	r.POST("/api/backups/restore", s.handleRestoreBackup)
	r.POST("/api/candidates/create", s.handleCreateCandidateDraft)
	r.POST("/api/chapter-report/export", s.handleExportChapterBundle)
	r.POST("/api/manuscript/export", s.handleExportManuscript)
	r.POST("/api/history/delete", s.handleDeleteHistoryEntry)
	r.POST("/api/history/export", s.handleExportHistory)
	r.POST("/api/settings", s.handleSaveSettings)
	r.POST("/api/projects", s.handleCreateProject)
	r.POST("/api/projects/:name/switch", s.handleSwitchProject)
	r.POST("/api/emotion-curve", s.handleEmotionCurve)
	r.POST("/check/stream", s.handleCheckStream)
	r.POST("/chat/stream", s.handleChatStream)
	r.POST("/rewrite/stream", s.handleRewriteStream)
	r.POST("/api/templates/apply", s.handleApplyTemplate)
	r.POST("/api/writeback/timeline", s.handleWritebackTimeline)
	r.POST("/api/writeback/foreshadow", s.handleWritebackForeshadow)
	r.POST("/api/writeback/relationship", s.handleWritebackRelationship)

	r.POST("/relationships", s.handleAddRelationship)
	r.POST("/relationships/delete", s.handleDeleteRelationship)

	r.POST("/timeline", s.handleAddTimelineEvent)
	r.POST("/timeline/delete", s.handleDeleteTimelineEvent)

	r.POST("/foreshadow", s.handleAddForeshadow)
	r.POST("/foreshadow/resolve", s.handleResolveForeshadow)
	r.POST("/foreshadow/delete", s.handleDeleteForeshadow)

	r.POST("/export", s.handleExport)
}

func (s *Server) loadProjectState(name string) (*projectState, error) {
	dataDir := workspace.ProjectDataDir(name)

	st := &projectState{dataDir: dataDir}
	st.project = projectsettings.New(
		filepath.Join(dataDir, "project_settings.json"),
		projectsettings.Settings{
			OllamaURL:  s.cfg.OllamaURL,
			LLMModel:   s.cfg.LLMModel,
			EmbedModel: s.cfg.EmbedModel,
			Port:       s.cfg.Port,
			DataDir:    dataDir,
		},
	)
	if err := st.project.Load(); err != nil {
		log.Printf("project settings load: %v", err)
	}

	st.profiles = profile.NewManager(dataDir)
	if err := st.profiles.Load(); err != nil {
		log.Printf("profiles load: %v", err)
	}

	st.store = vectorstore.New(filepath.Join(dataDir, "store.json"))
	if err := st.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}

	st.rules = reviewrules.New(filepath.Join(dataDir, "review_rules.json"))
	if err := st.rules.Load(); err != nil {
		log.Printf("review rules load: %v", err)
	}

	st.history = reviewhistory.New(filepath.Join(dataDir, "reviews.json"))
	if err := st.history.Load(); err != nil {
		log.Printf("review history load: %v", err)
	}

	st.relationships = tracker.NewRelationshipTracker(filepath.Join(dataDir, "relationships.json"))
	if err := st.relationships.Load(); err != nil {
		log.Printf("relationships load: %v", err)
	}

	st.timeline = tracker.NewTimelineTracker(filepath.Join(dataDir, "timeline.json"))
	if err := st.timeline.Load(); err != nil {
		log.Printf("timeline load: %v", err)
	}

	st.foreshadow = tracker.NewForeshadowTracker(filepath.Join(dataDir, "foreshadow.json"))
	if err := st.foreshadow.Load(); err != nil {
		log.Printf("foreshadow load: %v", err)
	}

	return st, nil
}

func (s *Server) currentState() *projectState {
	s.stateMu.RLock()
	st := s.state
	s.stateMu.RUnlock()
	if st != nil {
		return st
	}
	if s.profiles == nil && s.store == nil && s.project == nil && s.rules == nil && s.history == nil &&
		s.relationships == nil && s.timeline == nil && s.foreshadow == nil {
		return nil
	}
	dataDir := s.cfg.DataDir
	if dataDir == "" {
		dataDir = "data"
	}
	return &projectState{
		dataDir:       dataDir,
		profiles:      s.profiles,
		store:         s.store,
		project:       s.project,
		rules:         s.rules,
		history:       s.history,
		relationships: s.relationships,
		timeline:      s.timeline,
		foreshadow:    s.foreshadow,
	}
}

func (s *Server) setProjectState(st *projectState) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = st
	s.profiles = st.profiles
	s.store = st.store
	s.project = st.project
	s.rules = st.rules
	s.history = st.history
	s.relationships = st.relationships
	s.timeline = st.timeline
	s.foreshadow = st.foreshadow
	s.cfg.DataDir = st.dataDir
}

func (s *Server) switchProject(name string) error {
	newState, err := s.loadProjectState(name)
	if err != nil {
		return err
	}
	s.setProjectState(newState)
	s.applyProjectSettings()
	return nil
}

func (s *Server) Ingest(ctx context.Context) error {
	st := s.currentState()
	if err := st.profiles.Load(); err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	st.store.Clear()

	for _, char := range st.profiles.Characters {
		vec, err := s.embedder.Embed(ctx, char.RawContent)
		if err != nil {
			return fmt.Errorf("embed %s: %w", char.Name, err)
		}
		st.store.Upsert(vectorstore.Document{
			ID:        "char_" + char.Name,
			Type:      "character",
			Content:   char.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed character: %s", char.Name)
	}

	for _, world := range st.profiles.Worlds {
		vec, err := s.embedder.Embed(ctx, world.RawContent)
		if err != nil {
			return fmt.Errorf("embed world %s: %w", world.Name, err)
		}
		st.store.Upsert(vectorstore.Document{
			ID:        "world_" + world.Name,
			Type:      "world",
			Content:   world.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed world: %s", world.Name)
	}

	for _, style := range st.profiles.Styles {
		vec, err := s.embedder.Embed(ctx, style.RawContent)
		if err != nil {
			return fmt.Errorf("embed style %s: %w", style.Name, err)
		}
		st.store.Upsert(vectorstore.Document{
			ID:        "style_" + style.Name,
			Type:      "style",
			Content:   style.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed style: %s", style.Name)
	}

	files, err := os.ReadDir(chapterDirFor(st.dataDir))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list chapters: %w", err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(chapterDirFor(st.dataDir), file.Name()))
		if err != nil {
			return fmt.Errorf("read chapter %s: %w", file.Name(), err)
		}
		vec, err := s.embedder.Embed(ctx, string(content))
		if err != nil {
			return fmt.Errorf("embed chapter %s: %w", file.Name(), err)
		}
		st.store.Upsert(vectorstore.Document{
			ID:        "chapter_" + file.Name(),
			Type:      "chapter",
			Content:   string(content),
			Embedding: vec,
		})
		log.Printf("indexed chapter: %s", file.Name())
	}

	return st.store.Save()
}

func (s *Server) Run() error {
	return s.router.Run(":" + s.cfg.Port)
}

func (s *Server) applyProjectSettings() {
	st := s.currentState()
	if st == nil || st.project == nil {
		return
	}
	settings := st.project.Get()
	s.cfg.OllamaURL = settings.OllamaURL
	s.cfg.LLMModel = settings.LLMModel
	s.cfg.EmbedModel = settings.EmbedModel
	s.cfg.Port = settings.Port
	s.cfg.DataDir = st.dataDir
	s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)
	s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)
}
