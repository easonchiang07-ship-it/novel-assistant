package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"novel-assistant/internal/checker"
	"novel-assistant/internal/config"
	"novel-assistant/internal/consistency"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"
	"novel-assistant/internal/workspace"
	"novel-assistant/internal/worldstate"
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
	worldstate    *worldstate.Store
}

type Server struct {
	cfg            *config.Config
	globalDataDir  string
	router         *gin.Engine
	auth           *authManager
	diagnostics    *retrievalDiagnostics
	stateMu        sync.RWMutex
	state          *projectState
	profiles       *profile.Manager
	store          *vectorstore.Store
	project        *projectsettings.Store
	embedder       *embedder.OllamaEmbedder
	checker        *checker.Checker
	consistency    *consistency.Checker
	rules          *reviewrules.Store
	history        *reviewhistory.Store
	relationships  *tracker.RelationshipTracker
	timeline       *tracker.TimelineTracker
	foreshadow     *tracker.ForeshadowTracker
	worldstate     *worldstate.Store
	chapterOrderMu sync.RWMutex
	scenePlansMu   sync.RWMutex
}

func New(cfg *config.Config) (*Server, error) {
	idx, err := workspace.EnsureIndex()
	if err != nil {
		return nil, fmt.Errorf("load workspace index: %w", err)
	}

	s := &Server{
		cfg:           cfg,
		globalDataDir: cfg.DataDir,
		embedder:      embedder.New(cfg.OllamaURL, cfg.EmbedModel),
		checker:       checker.New(cfg.OllamaURL, cfg.LLMModel),
		auth:          newAuthManager(cfg),
		diagnostics:   newRetrievalDiagnostics(),
	}
	s.consistency = consistency.New(s.checker)
	if s.globalDataDir == "" {
		s.globalDataDir = "data"
	}

	s.project = projectsettings.New(filepath.Join(s.globalDataDir, "project_settings.json"), projectsettings.Settings{
		OllamaURL:  cfg.OllamaURL,
		LLMModel:   cfg.LLMModel,
		EmbedModel: cfg.EmbedModel,
		Port:       cfg.Port,
		DataDir:    s.globalDataDir,
	})
	if err := s.project.Load(); err != nil {
		log.Printf("project settings load: %v", err)
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
		"authEnabled": func() bool {
			return s.auth != nil && s.auth.Enabled()
		},
	})
	s.router.LoadHTMLGlob("web/templates/*.html")
	s.router.Static("/static", "web/static")
	s.setupRoutes()

	return s, nil
}

func (s *Server) setupRoutes() {
	r := s.router

	r.GET("/login", s.handleLoginPage)
	r.POST("/login", s.handleLogin)
	r.POST("/logout", s.handleLogout)

	protected := r.Group("/")
	protected.Use(s.requireAuth())

	protected.GET("/", s.handleIndex)
	protected.GET("/chapters", s.handleChaptersPage)
	protected.GET("/characters", s.handleCharacters)
	protected.GET("/history", s.handleHistoryPage)
	protected.GET("/settings", s.handleSettingsPage)
	protected.GET("/styles", s.handleStylesPage)
	protected.GET("/check", s.handleCheckPage)
	protected.GET("/diagnostics", s.handleDiagnosticsPage)
	protected.GET("/chat", s.handleChatPage)
	protected.GET("/relationships", s.handleRelationshipsPage)
	protected.GET("/timeline", s.handleTimelinePage)
	protected.GET("/foreshadow", s.handleForeshadowPage)
	protected.GET("/api/history/:id", s.handleGetHistoryEntry)
	protected.GET("/api/history/:id/diff", s.handleGetHistoryDiff)
	protected.GET("/api/backups", s.handleListBackups)
	protected.GET("/api/backups/:name/preview", s.handleGetBackupPreview)
	protected.GET("/api/projects", s.handleListProjects)
	protected.GET("/api/chapters/:name/analysis", s.handleAnalyzeChapter)
	protected.GET("/api/chapters", s.handleListChapters)
	protected.GET("/api/chapters/:name", s.handleGetChapter)
	protected.GET("/api/settings", s.handleGetSettings)
	protected.GET("/api/worldstate", s.handleListWorldstate)

	protected.POST("/ingest", s.handleIngest)
	protected.POST("/api/chapters", s.handleSaveChapter)
	protected.POST("/api/chapters/:name/snapshot", s.handleCreateWorldstateSnapshot)
	protected.POST("/api/chapters/order", s.handleSaveChapterOrder)
	protected.POST("/api/chapters/:name/scenes/plan", s.handleSaveScenePlan)
	protected.POST("/api/chapters/:name/scenes/order", s.handleSaveSceneOrder)
	protected.POST("/api/backups/create", s.handleCreateBackup)
	protected.POST("/api/backups/restore", s.handleRestoreBackup)
	protected.POST("/api/candidates/create", s.handleCreateCandidateDraft)
	protected.POST("/api/chapter-report/export", s.handleExportChapterBundle)
	protected.POST("/api/manuscript/export", s.handleExportManuscript)
	protected.POST("/api/history/delete", s.handleDeleteHistoryEntry)
	protected.POST("/api/history/export", s.handleExportHistory)
	protected.POST("/api/styles/analyze", s.handleAnalyzeStyle)
	protected.POST("/api/styles/:name/analysis", s.handleApplyStyleAnalysis)
	protected.POST("/api/settings", s.handleSaveSettings)
	protected.POST("/api/projects", s.handleCreateProject)
	protected.POST("/api/projects/:name/switch", s.handleSwitchProject)
	protected.POST("/api/emotion-curve", s.handleEmotionCurve)
	protected.POST("/check/stream", s.handleCheckStream)
	protected.POST("/chat/stream", s.handleChatStream)
	protected.POST("/rewrite/stream", s.handleRewriteStream)
	protected.POST("/api/templates/apply", s.handleApplyTemplate)
	protected.POST("/api/writeback/timeline", s.handleWritebackTimeline)
	protected.POST("/api/writeback/foreshadow", s.handleWritebackForeshadow)
	protected.POST("/api/writeback/relationship", s.handleWritebackRelationship)

	protected.POST("/relationships", s.handleAddRelationship)
	protected.POST("/relationships/delete", s.handleDeleteRelationship)

	protected.POST("/timeline", s.handleAddTimelineEvent)
	protected.POST("/timeline/delete", s.handleDeleteTimelineEvent)

	protected.POST("/foreshadow", s.handleAddForeshadow)
	protected.POST("/foreshadow/resolve", s.handleResolveForeshadow)
	protected.POST("/foreshadow/delete", s.handleDeleteForeshadow)
	protected.POST("/foreshadow/detect", s.handleDetectForeshadow)
	protected.POST("/foreshadow/confirm", s.handleConfirmForeshadow)
	protected.POST("/foreshadow/dismiss", s.handleDismissForeshadow)
	protected.GET("/foreshadow/stale", s.handleStaleForeshadow)

	protected.GET("/narrative", s.handleNarrativePage)
	protected.POST("/narrative/extract", s.handleNarrativeExtract)

	protected.GET("/evaluate", s.handleEvaluatePage)
	protected.POST("/evaluate/stream", s.handleEvaluateStream)
	protected.GET("/api/demo", s.handleDemoData)
	protected.GET("/api/ollama/status", s.handleOllamaStatus)
	protected.POST("/api/ollama/pull", s.handleOllamaPull)

	protected.POST("/export", s.handleExport)
}

func (s *Server) loadProjectState(name string) (*projectState, error) {
	dataDir := workspace.ProjectDataDir(name)

	st := &projectState{dataDir: dataDir}
	st.project = s.project

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

	st.worldstate = worldstate.New(filepath.Join(dataDir, "worldstate.json"))
	if err := st.worldstate.Load(); err != nil {
		log.Printf("worldstate load: %v", err)
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
		s.relationships == nil && s.timeline == nil && s.foreshadow == nil && s.worldstate == nil {
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
		worldstate:    s.worldstate,
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
	s.worldstate = st.worldstate
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
		chapterText := string(content)
		chapterIdx := extractChapterIndex(file.Name())
		chunks := chunkChapter(file.Name(), chapterText)
		for _, chunk := range chunks {
			vec, err := s.embedder.Embed(ctx, chunk.Content)
			if err != nil {
				return fmt.Errorf("embed chapter chunk %s: %w", chunk.ID, err)
			}
			chunk.Embedding = vec
			st.store.Upsert(chunk)
			log.Printf("indexed chapter chunk: %s", chunk.ID)
		}
		if len(chunks) == 0 {
			continue
		}
		summary, err := s.checker.SummarizeChapter(ctx, chapterText)
		if err != nil {
			log.Printf("summarize chapter %s: %v (skipped)", file.Name(), err)
			continue
		}
		summaryVec, err := s.embedder.Embed(ctx, summary)
		if err != nil {
			log.Printf("embed summary %s: %v (skipped)", file.Name(), err)
			continue
		}
		st.store.Upsert(vectorstore.Document{
			ID:           "summary_" + file.Name(),
			Type:         "chapter_summary",
			Content:      summary,
			Summary:      summary,
			Embedding:    summaryVec,
			ChapterFile:  file.Name(),
			ChapterIndex: chapterIdx,
		})
		log.Printf("indexed chapter summary: %s", file.Name())
	}

	return st.store.Save()
}

func (s *Server) Run() error {
	return s.router.Run(":" + s.cfg.Port)
}

func (s *Server) applyProjectSettings() {
	if s.project == nil {
		return
	}
	settings := s.project.Get()
	s.cfg.OllamaURL = settings.OllamaURL
	s.cfg.LLMModel = settings.LLMModel
	s.cfg.EmbedModel = settings.EmbedModel
	s.cfg.Port = settings.Port
	if st := s.currentState(); st != nil {
		s.cfg.DataDir = st.dataDir
	}
	s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)
	s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)
	s.consistency = consistency.New(s.checker)
}
