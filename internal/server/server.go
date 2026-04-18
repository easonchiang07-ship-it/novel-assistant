package server

import (
	"context"
	"fmt"
	"log"
	"novel-assistant/internal/checker"
	"novel-assistant/internal/config"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"

	"github.com/gin-gonic/gin"
)

type Server struct {
	cfg           *config.Config
	router        *gin.Engine
	profiles      *profile.Manager
	store         *vectorstore.Store
	embedder      *embedder.OllamaEmbedder
	checker       *checker.Checker
	relationships *tracker.RelationshipTracker
	timeline      *tracker.TimelineTracker
	foreshadow    *tracker.ForeshadowTracker
}

func New(cfg *config.Config) (*Server, error) {
	s := &Server{cfg: cfg}

	s.profiles = profile.NewManager(cfg.DataDir)
	if err := s.profiles.Load(); err != nil {
		log.Printf("profiles load: %v", err)
	}

	s.store = vectorstore.New(cfg.DataDir + "/store.json")
	if err := s.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}

	s.embedder = embedder.New(cfg.OllamaURL, cfg.EmbedModel)
	s.checker = checker.New(cfg.OllamaURL, cfg.LLMModel)

	s.relationships = tracker.NewRelationshipTracker(cfg.DataDir + "/relationships.json")
	if err := s.relationships.Load(); err != nil {
		log.Printf("relationships load: %v", err)
	}

	s.timeline = tracker.NewTimelineTracker(cfg.DataDir + "/timeline.json")
	if err := s.timeline.Load(); err != nil {
		log.Printf("timeline load: %v", err)
	}

	s.foreshadow = tracker.NewForeshadowTracker(cfg.DataDir + "/foreshadow.json")
	if err := s.foreshadow.Load(); err != nil {
		log.Printf("foreshadow load: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	s.router = gin.Default()
	s.router.LoadHTMLGlob("web/templates/*.html")
	s.router.Static("/static", "web/static")
	s.setupRoutes()

	return s, nil
}

func (s *Server) setupRoutes() {
	r := s.router

	r.GET("/", s.handleIndex)
	r.GET("/characters", s.handleCharacters)
	r.GET("/check", s.handleCheckPage)
	r.GET("/relationships", s.handleRelationshipsPage)
	r.GET("/timeline", s.handleTimelinePage)
	r.GET("/foreshadow", s.handleForeshadowPage)

	r.POST("/ingest", s.handleIngest)
	r.POST("/check/stream", s.handleCheckStream)

	r.POST("/relationships", s.handleAddRelationship)
	r.POST("/relationships/delete", s.handleDeleteRelationship)

	r.POST("/timeline", s.handleAddTimelineEvent)
	r.POST("/timeline/delete", s.handleDeleteTimelineEvent)

	r.POST("/foreshadow", s.handleAddForeshadow)
	r.POST("/foreshadow/resolve", s.handleResolveForeshadow)
	r.POST("/foreshadow/delete", s.handleDeleteForeshadow)

	r.POST("/export", s.handleExport)
}

func (s *Server) Ingest(ctx context.Context) error {
	if err := s.profiles.Load(); err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	s.store.Clear()

	for _, char := range s.profiles.Characters {
		vec, err := s.embedder.Embed(ctx, char.RawContent)
		if err != nil {
			return fmt.Errorf("embed %s: %w", char.Name, err)
		}
		s.store.Upsert(vectorstore.Document{
			ID:        "char_" + char.Name,
			Type:      "character",
			Content:   char.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed character: %s", char.Name)
	}

	for _, world := range s.profiles.Worlds {
		vec, err := s.embedder.Embed(ctx, world.RawContent)
		if err != nil {
			return fmt.Errorf("embed world %s: %w", world.Name, err)
		}
		s.store.Upsert(vectorstore.Document{
			ID:        "world_" + world.Name,
			Type:      "world",
			Content:   world.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed world: %s", world.Name)
	}

	for _, style := range s.profiles.Styles {
		vec, err := s.embedder.Embed(ctx, style.RawContent)
		if err != nil {
			return fmt.Errorf("embed style %s: %w", style.Name, err)
		}
		s.store.Upsert(vectorstore.Document{
			ID:        "style_" + style.Name,
			Type:      "style",
			Content:   style.RawContent,
			Embedding: vec,
		})
		log.Printf("indexed style: %s", style.Name)
	}

	return s.store.Save()
}

func (s *Server) Run() error {
	return s.router.Run(":" + s.cfg.Port)
}
