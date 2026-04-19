package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"novel-assistant/internal/checker"
	"novel-assistant/internal/exporter"
	"novel-assistant/internal/extractor"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type streamEvent struct {
	Event     string
	Text      string
	Sources   []referenceSummary
	Retrieval any
	Gaps      *retrievalGaps
}

type referenceSummary struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Excerpt      string  `json:"excerpt"`
	Score        float64 `json:"score"`
	MatchReason  string  `json:"match_reason"`
	ChapterMatch string  `json:"chapter_match"`
}

type retrievalSummary struct {
	Task      string   `json:"task"`
	Sources   []string `json:"sources"`
	TopK      int      `json:"top_k"`
	Threshold float64  `json:"threshold"`
}

type retrievalGaps struct {
	IndexReady        bool     `json:"index_ready"`
	MissingCharacters []string `json:"missing_characters"`
	MissingLocations  []string `json:"missing_locations"`
	MissingSettings   []string `json:"missing_settings"`
}

type chanWriter struct {
	ch         chan<- streamEvent
	transcript *strings.Builder
}

func (cw *chanWriter) Write(p []byte) (n int, err error) {
	if cw.transcript != nil {
		cw.transcript.Write(p)
	}
	cw.ch <- streamEvent{Event: "chunk", Text: string(p)}
	return len(p), nil
}

type flushWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err == nil && fw.flusher != nil {
		fw.flusher.Flush()
	}
	return n, err
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func parsePositiveChapter(raw string) (int, error) {
	chapter, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || chapter < 1 {
		return 0, fmt.Errorf("章節必須是大於 0 的整數")
	}
	return chapter, nil
}

func saveOrAbort(c *gin.Context, err error, action string) bool {
	if err == nil {
		return true
	}
	log.Printf("%s: %v", action, err)
	c.String(http.StatusInternalServerError, "%s", "資料保存失敗，請稍後再試")
	return false
}

func reviewBiasInstruction(mode string) string {
	switch mode {
	case "strict":
		return "偏挑錯模式：優先指出矛盾、違和與模糊處，語氣直接，少做保留。"
	case "coaching":
		return "偏修稿建議模式：除了指出問題，也請提供具體修改方向與可執行建議。"
	case "conservative":
		return "偏保守模式：只有在問題明顯時才指出，避免過度挑剔。"
	default:
		return "平衡模式：兼顧指出問題與肯定有效之處。"
	}
}

func rewriteBiasInstruction(mode string) string {
	switch mode {
	case "expressive":
		return "本次修稿偏好：在不破壞原意的前提下，可以適度加強文氣、意象與敘述張力。"
	case "structural":
		return "本次修稿偏好：優先整理段落結構、節奏與資訊揭露順序。"
	default:
		return "本次修稿偏好：盡量忠於原稿，只做必要調整。"
	}
}

func joinProfiles(items []vectorProfile) string {
	if len(items) == 0 {
		return ""
	}

	var parts []string
	for _, item := range items {
		parts = append(parts, item.Content)
	}
	return strings.Join(parts, "\n\n")
}

type vectorProfile struct {
	Name         string
	Type         string
	Content      string
	Score        float64
	MatchReason  string
	ChapterMatch string
}

func excerptText(content string) string {
	compacted := strings.Join(strings.Fields(strings.ReplaceAll(content, "\n", " ")), " ")
	if len(compacted) <= 120 {
		return compacted
	}
	return compacted[:120] + "..."
}

func mergeRetrieval(preset reviewrules.RetrievalPreset, override retrievalOptions) retrievalOptions {
	result := retrievalOptions{
		Sources:   append([]string(nil), preset.Sources...),
		TopK:      preset.TopK,
		Threshold: preset.Threshold,
	}
	if len(override.Sources) > 0 {
		result.Sources = append([]string(nil), override.Sources...)
	}
	if override.TopK >= 1 {
		result.TopK = override.TopK
	}
	if override.ThresholdSet {
		result.Threshold = override.Threshold
	}
	return result
}

func summarizeRetrieval(task string, opts retrievalOptions) retrievalSummary {
	return retrievalSummary{
		Task:      task,
		Sources:   append([]string(nil), opts.Sources...),
		TopK:      opts.TopK,
		Threshold: opts.Threshold,
	}
}

func historyRetrievalConfig(opts retrievalSummary) reviewhistory.RetrievalConfig {
	return reviewhistory.RetrievalConfig{
		Sources:   append([]string(nil), opts.Sources...),
		TopK:      opts.TopK,
		Threshold: opts.Threshold,
	}
}

func buildHistoryRetrievalConfigs(active map[string]retrievalSummary) map[string]reviewhistory.RetrievalConfig {
	result := make(map[string]reviewhistory.RetrievalConfig, len(active))
	for task, summary := range active {
		if summary.TopK > 0 {
			result[task] = historyRetrievalConfig(summary)
		}
	}
	return result
}

func summarizeReferences(items []vectorProfile) []referenceSummary {
	summaries := make([]referenceSummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, referenceSummary{
			Name:         item.Name,
			Type:         item.Type,
			Excerpt:      excerptText(item.Content),
			Score:        item.Score,
			MatchReason:  item.MatchReason,
			ChapterMatch: item.ChapterMatch,
		})
	}
	return summaries
}

func referenceNames(items []vectorProfile) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s:%s", item.Type, item.Name))
	}
	return names
}

func filterReferencesByType(items []vectorProfile, refType string) []vectorProfile {
	var filtered []vectorProfile
	for _, item := range items {
		if item.Type == refType {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func computeRetrievalGaps(chapter string, knownNames []string, retrieved []vectorProfile) retrievalGaps {
	signals := extractor.AnalyzeChapter(chapter, knownNames)

	retrievedNames := make(map[string]struct{}, len(retrieved))
	for _, ref := range retrieved {
		retrievedNames[ref.Name] = struct{}{}
	}

	filterMissing := func(items []string) []string {
		out := make([]string, 0, len(items))
		for _, item := range items {
			if _, ok := retrievedNames[item]; ok {
				continue
			}
			out = append(out, item)
		}
		return out
	}

	return retrievalGaps{
		IndexReady:        false,
		MissingCharacters: filterMissing(signals.KnownCharacters),
		MissingLocations:  filterMissing(signals.LocationCandidates),
		MissingSettings:   filterMissing(signals.SettingCandidates),
	}
}

func mergeReferenceLists(groups ...[]vectorProfile) []vectorProfile {
	seen := make(map[string]struct{})
	merged := make([]vectorProfile, 0)
	for _, group := range groups {
		for _, item := range group {
			key := item.Type + "\x00" + item.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, item)
		}
	}
	return merged
}

func joinWorldProfiles(refs []vectorProfile, worlds []*profile.WorldSetting) string {
	if len(refs) > 0 {
		return joinProfiles(refs)
	}
	if len(worlds) == 0 {
		return ""
	}

	parts := make([]string, 0, len(worlds))
	for _, world := range worlds {
		parts = append(parts, world.RawContent)
	}
	return strings.Join(parts, "\n\n")
}

func (s *Server) resolveStyles(req checkRequest) ([]*profile.StyleGuide, error) {
	if !contains(req.Checks, "style") {
		return nil, nil
	}
	if len(s.profiles.Styles) == 0 {
		return nil, fmt.Errorf("尚無可用的寫作風格設定，請先在 data/style/ 新增 .md 檔並重新索引")
	}

	names := req.Styles
	if len(names) == 0 {
		names = s.profiles.AllStyleNames()
	}

	styles := make([]*profile.StyleGuide, 0, len(names))
	for _, styleName := range names {
		sg := s.profiles.FindStyleByName(styleName)
		if sg == nil {
			return nil, fmt.Errorf("找不到寫作風格：%s", styleName)
		}
		if strings.TrimSpace(sg.RawContent) == "" {
			return nil, fmt.Errorf("寫作風格內容不可為空：%s", styleName)
		}
		styles = append(styles, sg)
	}
	return styles, nil
}

func (s *Server) buildReferenceContext(ctx context.Context, chapter, chapterFile string, opts retrievalOptions) ([]vectorProfile, error) {
	if s.store.Len() == 0 {
		return nil, nil
	}

	queryVec, err := s.embedder.Embed(ctx, chapter)
	if err != nil {
		return nil, err
	}

	rules := s.rules.Get()
	topK := opts.TopK
	if topK < 1 {
		topK = rules.RetrievalTopK
	}
	sources := opts.Sources
	if len(sources) == 0 {
		sources = rules.RetrievalSources
	}
	threshold := opts.Threshold
	if threshold < 0 || threshold > 1 {
		threshold = rules.RetrievalThreshold
	}

	docs := s.store.QueryFilteredScored(queryVec, topK, sources, threshold)
	results := make([]vectorProfile, 0, len(docs))
	for _, doc := range docs {
		if doc.Type == "chapter" && strings.TrimSpace(chapterFile) != "" && doc.ID == "chapter_"+chapterFile {
			continue
		}
		name := strings.TrimPrefix(doc.ID, "char_")
		name = strings.TrimPrefix(name, "world_")
		name = strings.TrimPrefix(name, "style_")
		name = strings.TrimPrefix(name, "chapter_")
		reason, snippet := referenceMatchDetail(chapter, name, doc.Content)
		results = append(results, vectorProfile{
			Name:         name,
			Type:         doc.Type,
			Content:      doc.Content,
			Score:        doc.Score,
			MatchReason:  reason,
			ChapterMatch: snippet,
		})
	}
	return results, nil
}

func referenceMatchDetail(chapter, name, content string) (string, string) {
	if snippet := chapterSnippetAround(chapter, name); snippet != "" {
		return "章節直接提到此參考名稱", snippet
	}

	keywords := extractReferenceKeywords(content)
	for _, keyword := range keywords {
		if snippet := chapterSnippetAround(chapter, keyword); snippet != "" {
			return fmt.Sprintf("章節命中參考關鍵詞「%s」", keyword), snippet
		}
	}

	return "由向量相似度命中此參考", excerptText(chapter)
}

func chapterSnippetAround(chapter, needle string) string {
	chapter = strings.TrimSpace(chapter)
	needle = strings.TrimSpace(needle)
	if chapter == "" || needle == "" {
		return ""
	}

	idx := strings.Index(chapter, needle)
	if idx < 0 {
		return ""
	}

	start := idx - 24
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 24
	if end > len(chapter) {
		end = len(chapter)
	}
	snippet := strings.TrimSpace(chapter[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(chapter) {
		snippet += "..."
	}
	return snippet
}

func extractReferenceKeywords(content string) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '：' || r == ':' || r == '、' || r == '，' || r == ',' || r == '-' || r == '。'
	})

	seen := make(map[string]struct{})
	keywords := make([]string, 0, 6)
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if len([]rune(trimmed)) < 2 {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimPrefix(trimmed, "#")
			trimmed = strings.TrimSpace(trimmed)
		}
		if len([]rune(trimmed)) < 2 {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		keywords = append(keywords, trimmed)
		if len(keywords) == 6 {
			break
		}
	}
	return keywords
}

func (s *Server) resolveCharacters(req checkRequest) []*profile.Character {
	names := req.Characters
	if len(names) == 0 {
		names = checker.ExtractNames(req.Chapter, s.profiles.AllNames())
	}
	if len(names) == 0 {
		names = s.profiles.AllNames()
	}

	chars := make([]*profile.Character, 0, len(names))
	for _, charName := range names {
		if char := s.profiles.FindByName(charName); char != nil {
			chars = append(chars, char)
		}
	}
	return chars
}

// ─── pages ───────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(c *gin.Context) {
	open := 0
	for _, f := range s.foreshadow.GetAll() {
		if f.Status == "未回收" {
			open++
		}
	}
	chapters, err := s.listChapterFiles()
	if err != nil {
		log.Printf("list chapters: %v", err)
	}
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Title":          "儀表板",
		"CharCount":      len(s.profiles.Characters),
		"WorldCount":     len(s.profiles.Worlds),
		"StyleCount":     len(s.profiles.Styles),
		"ChapterCount":   len(chapters),
		"RelCount":       len(s.relationships.GetAll()),
		"EventCount":     len(s.timeline.GetSorted()),
		"ForeshadowOpen": open,
		"HistoryCount":   len(s.history.Recent(200)),
		"VectorCount":    s.store.Len(),
	})
}

func (s *Server) handleCharacters(c *gin.Context) {
	c.HTML(http.StatusOK, "characters.html", gin.H{
		"Title":      "角色管理",
		"Characters": s.profiles.Characters,
		"Worlds":     s.profiles.Worlds,
	})
}

func (s *Server) handleStylesPage(c *gin.Context) {
	c.HTML(http.StatusOK, "styles.html", gin.H{
		"Title":  "風格管理",
		"Styles": s.profiles.Styles,
	})
}

func (s *Server) handleCheckPage(c *gin.Context) {
	chapters, err := s.listChapterFiles()
	if err != nil {
		log.Printf("list chapters: %v", err)
	}
	rules := s.rules.Get()
	knownCharacterNames := make([]string, 0, len(s.profiles.Characters))
	for _, ch := range s.profiles.Characters {
		knownCharacterNames = append(knownCharacterNames, ch.Name)
	}
	knownWorldNames := make([]string, 0, len(s.profiles.Worlds))
	for _, world := range s.profiles.Worlds {
		knownWorldNames = append(knownWorldNames, world.Name)
	}
	c.HTML(http.StatusOK, "check.html", gin.H{
		"Title":               "一致性審查",
		"Characters":          s.profiles.Characters,
		"Styles":              s.profiles.Styles,
		"Chapters":            chapters,
		"DefaultChecks":       rules.DefaultChecks,
		"DefaultStyles":       rules.DefaultStyles,
		"ReviewBias":          rules.ReviewBias,
		"RewriteBias":         rules.RewriteBias,
		"RetrievalSources":    rules.RetrievalSources,
		"RetrievalTopK":       rules.RetrievalTopK,
		"RetrievalThreshold":  rules.RetrievalThreshold,
		"KnownCharacterNames": knownCharacterNames,
		"KnownWorldNames":     knownWorldNames,
	})
}

func (s *Server) handleEmotionCurve(c *gin.Context) {
	var req struct {
		Chapter string `json:"chapter"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	points, err := s.checker.AnalyzeEmotionCurve(c.Request.Context(), req.Chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"points": points})
}

func (s *Server) handleChatPage(c *gin.Context) {
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"Title":      "角色對談室",
		"Characters": s.profiles.Characters,
	})
}

func (s *Server) handleRelationshipsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "relationships.html", gin.H{
		"Title":         "關係圖",
		"Relationships": s.relationships.GetAll(),
		"Characters":    s.profiles.AllNames(),
	})
}

func (s *Server) handleTimelinePage(c *gin.Context) {
	c.HTML(http.StatusOK, "timeline.html", gin.H{
		"Title":      "時間軸",
		"Events":     s.timeline.GetSorted(),
		"Characters": s.profiles.AllNames(),
	})
}

func (s *Server) handleForeshadowPage(c *gin.Context) {
	c.HTML(http.StatusOK, "foreshadow.html", gin.H{
		"Title":       "伏筆追蹤",
		"Foreshadows": s.foreshadow.GetAll(),
	})
}

// ─── ingest ───────────────────────────────────────────────────────────────────

func (s *Server) handleIngest(c *gin.Context) {
	ctx := c.Request.Context()
	if err := s.Ingest(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":    "索引完成",
		"characters": len(s.profiles.Characters),
		"worlds":     len(s.profiles.Worlds),
	})
}

// ─── check stream ─────────────────────────────────────────────────────────────

type checkRequest struct {
	Chapter            string                      `json:"chapter"`
	Characters         []string                    `json:"characters"`
	Checks             []string                    `json:"checks"` // ["behavior","dialogue","style","world"]
	Styles             []string                    `json:"styles"` // style guide names to apply; empty = all styles
	Retrieval          retrievalOptions            `json:"retrieval"`
	RetrievalOverrides map[string]retrievalOptions `json:"retrieval_overrides,omitempty"`
	ChapterFile        string                      `json:"chapter_file"`
	ChapterTitle       string                      `json:"chapter_title"`
	SceneTitle         string                      `json:"scene_title,omitempty"` // empty = full chapter
}

type retrievalOptions struct {
	Sources      []string `json:"sources"`
	TopK         int      `json:"top_k"`
	Threshold    float64  `json:"threshold"`
	ThresholdSet bool     `json:"threshold_set,omitempty"`
}

func (r checkRequest) retrievalOverrideFor(task string) retrievalOptions {
	if override, ok := r.RetrievalOverrides[task]; ok {
		return override
	}
	return r.Retrieval
}

func (s *Server) handleCheckStream(c *gin.Context) {
	var req checkRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	if _, err := s.resolveStyles(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msgChan := make(chan streamEvent, 512)
	ctx, cancel := context.WithCancel(c.Request.Context())
	var transcript strings.Builder
	reviewBias := reviewBiasInstruction(s.rules.Get().ReviewBias)

	go func() {
		defer cancel()
		defer close(msgChan)

		cw := &chanWriter{ch: msgChan, transcript: &transcript}
		charsToCheck := s.resolveCharacters(req)
		needsCharacters := len(req.Checks) == 0 || contains(req.Checks, "behavior") || contains(req.Checks, "dialogue")
		if needsCharacters && len(charsToCheck) == 0 {
			text := "\n> 找不到可審查的角色，請先建立角色設定檔。\n"
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
			return
		}

		var behaviorRefs []vectorProfile
		var dialogueRefs []vectorProfile
		var worldRefs []vectorProfile
		activeRetrieval := make(map[string]retrievalSummary)
		var err error

		if len(req.Checks) == 0 || contains(req.Checks, "behavior") {
			behaviorOpts := mergeRetrieval(s.rules.PresetFor("behavior"), req.retrievalOverrideFor("behavior"))
			activeRetrieval["behavior"] = summarizeRetrieval("behavior", behaviorOpts)
			behaviorRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, behaviorOpts)
			if err != nil {
				text := fmt.Sprintf("\n> 行為審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}
		if contains(req.Checks, "dialogue") {
			dialogueOpts := mergeRetrieval(s.rules.PresetFor("dialogue"), req.retrievalOverrideFor("dialogue"))
			activeRetrieval["dialogue"] = summarizeRetrieval("dialogue", dialogueOpts)
			dialogueRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, dialogueOpts)
			if err != nil {
				text := fmt.Sprintf("\n> 對白審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}
		if contains(req.Checks, "world") {
			worldOpts := mergeRetrieval(s.rules.PresetFor("world"), req.retrievalOverrideFor("world"))
			activeRetrieval["world"] = summarizeRetrieval("world", worldOpts)
			worldRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, worldOpts)
			if err != nil {
				text := fmt.Sprintf("\n> 世界觀審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}

		msgChan <- streamEvent{Event: "retrieval", Retrieval: gin.H{"tasks": activeRetrieval}}

		references := mergeReferenceLists(behaviorRefs, dialogueRefs, worldRefs)
		indexReady := s.store != nil && s.store.Len() > 0
		if len(references) > 0 {
			msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
			gaps := computeRetrievalGaps(req.Chapter, s.profiles.AllNames(), references)
			gaps.IndexReady = indexReady
			if len(gaps.MissingCharacters)+len(gaps.MissingLocations)+len(gaps.MissingSettings) > 0 {
				msgChan <- streamEvent{Event: "gaps", Gaps: &gaps}
			}
			transcript.WriteString("### 本地參考上下文\n\n")
			msgChan <- streamEvent{Event: "chunk", Text: "### 本地參考上下文\n\n"}
			for _, ref := range references {
				text := fmt.Sprintf("- [%s] %s：%s\n", ref.Type, ref.Name, excerptText(ref.Content))
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
			transcript.WriteString("\n")
			msgChan <- streamEvent{Event: "chunk", Text: "\n"}
		} else {
			msgChan <- streamEvent{Event: "sources", Sources: nil}
			gaps := computeRetrievalGaps(req.Chapter, s.profiles.AllNames(), nil)
			gaps.IndexReady = indexReady
			if len(gaps.MissingCharacters)+len(gaps.MissingLocations)+len(gaps.MissingSettings) > 0 {
				msgChan <- streamEvent{Event: "gaps", Gaps: &gaps}
			}
		}

		behaviorRefText := joinProfiles(behaviorRefs)
		dialogueRefText := joinProfiles(dialogueRefs)
		worldText := joinWorldProfiles(filterReferencesByType(worldRefs, "world"), s.profiles.Worlds)
		if contains(req.Checks, "world") {
			if strings.TrimSpace(worldText) == "" {
				text := "\n> 尚無世界觀設定可供審查，請先新增 worldbuilding 檔案或重新索引。\n"
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			} else {
				transcript.WriteString("\n\n## 世界觀衝突審查\n\n")
				msgChan <- streamEvent{Event: "chunk", Text: "\n\n## 世界觀衝突審查\n\n"}
				worldPrompt := worldText + "\n\n【審查偏好】\n" + reviewBias
				if err := s.checker.CheckWorldConflictStream(ctx, worldPrompt, req.Chapter, cw); err != nil {
					if ctx.Err() == nil {
						text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
						transcript.WriteString(text)
						msgChan <- streamEvent{Event: "chunk", Text: text}
					}
					return
				}
			}
		}

		if contains(req.Checks, "opening") {
			text := "\n\n## 黃金三章診斷\n\n"
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
			if err := s.checker.DiagnoseOpeningStream(ctx, req.Chapter, cw); err != nil {
				if ctx.Err() == nil {
					log.Printf("opening diagnosis: %v", err)
					text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
					transcript.WriteString(text)
					msgChan <- streamEvent{Event: "chunk", Text: text}
				}
			}
		}

		for _, char := range charsToCheck {
			text := fmt.Sprintf("\n\n## 角色：%s\n\n", char.Name)
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}

			if len(req.Checks) == 0 || contains(req.Checks, "behavior") {
				transcript.WriteString("### 行為一致性審查\n\n")
				msgChan <- streamEvent{Event: "chunk", Text: "### 行為一致性審查\n\n"}
				profileText := char.RawContent
				profileText += "\n\n【審查偏好】\n" + reviewBias
				if behaviorRefText != "" {
					profileText += "\n\n【補充參考資料】\n" + behaviorRefText
				}
				if err := s.checker.CheckBehaviorStream(ctx, profileText, req.Chapter, cw); err != nil {
					if ctx.Err() == nil {
						text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
						transcript.WriteString(text)
						msgChan <- streamEvent{Event: "chunk", Text: text}
					}
					return
				}
			}

			if contains(req.Checks, "dialogue") {
				transcript.WriteString("\n\n### 對白風格審查\n\n")
				msgChan <- streamEvent{Event: "chunk", Text: "\n\n### 對白風格審查\n\n"}
				dialogueStyle := char.SpeechStyle
				if dialogueStyle != "" {
					dialogueStyle += "；"
				}
				dialogueStyle += reviewBias
				if dialogueRefText != "" {
					dialogueStyle += "\n\n【補充參考資料】\n" + dialogueRefText
				}
				if err := s.checker.CheckDialogueStream(ctx, char.Name, char.Personality, dialogueStyle, req.Chapter, cw); err != nil {
					if ctx.Err() == nil {
						text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
						transcript.WriteString(text)
						msgChan <- streamEvent{Event: "chunk", Text: text}
					}
					return
				}
			}
		}

		// 寫作風格審查（獨立於角色，逐一套用所選風格）
		stylesToCheck, err := s.resolveStyles(req)
		if err != nil {
			text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
			return
		}
		for _, sg := range stylesToCheck {
			text := fmt.Sprintf("\n\n## 寫作風格：%s\n\n### 風格一致性審查\n\n", sg.Name)
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
			stylePrompt := sg.RawContent + "\n\n【審查偏好】\n" + reviewBias
			if err := s.checker.CheckStyleStream(ctx, stylePrompt, req.Chapter, cw); err != nil {
				if ctx.Err() == nil {
					text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
					transcript.WriteString(text)
					msgChan <- streamEvent{Event: "chunk", Text: text}
				}
				return
			}
		}

		completion := "\n\n---\n審查完成\n"
		msgChan <- streamEvent{Event: "chunk", Text: completion}
		transcript.WriteString(completion)

		chapterFile := strings.TrimSpace(req.ChapterFile)
		chapterTitle := strings.TrimSpace(req.ChapterTitle)
		if chapterTitle == "" && chapterFile != "" {
			chapterTitle = strings.TrimSuffix(chapterFile, ".md")
		}
		if chapterTitle == "" {
			chapterTitle = "未命名章節"
		}
		s.history.Add(&reviewhistory.Entry{
			Kind:             "review",
			ChapterTitle:     chapterTitle,
			ChapterFile:      chapterFile,
			SceneTitle:       strings.TrimSpace(req.SceneTitle),
			InputContent:     req.Chapter,
			Checks:           append([]string(nil), req.Checks...),
			Styles:           append([]string(nil), req.Styles...),
			Sources:          referenceNames(references),
			RetrievalConfigs: buildHistoryRetrievalConfigs(activeRetrieval),
			Result:           transcript.String(),
		})
		if err := s.history.Save(); err != nil {
			log.Printf("save review history: %v", err)
		}
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return false
			}
			if msg.Event == "sources" {
				c.SSEvent("sources", gin.H{"items": msg.Sources})
				return true
			}
			if msg.Event == "retrieval" {
				c.SSEvent("retrieval", msg.Retrieval)
				return true
			}
			if msg.Event == "gaps" {
				c.SSEvent("gaps", msg.Gaps)
				return true
			}
			c.SSEvent("chunk", gin.H{"text": msg.Text})
			return true
		case <-ctx.Done():
			return false
		}
	})
}

type rewriteRequest struct {
	Chapter      string           `json:"chapter"`
	Mode         string           `json:"mode"`
	Characters   []string         `json:"characters"`
	Styles       []string         `json:"styles"`
	Retrieval    retrievalOptions `json:"retrieval"`
	ChapterFile  string           `json:"chapter_file"`
	ChapterTitle string           `json:"chapter_title"`
	SceneTitle   string           `json:"scene_title,omitempty"` // empty = full chapter
}

type chatRequest struct {
	CharacterName string `json:"character_name"`
	History       string `json:"history"`
	Message       string `json:"message"`
}

func rewriteInstruction(mode string) (string, error) {
	switch mode {
	case "conservative":
		return "請做保守修訂：保留原本情節與語意，只修正違和、措辭與局部節奏。", nil
	case "style":
		return "請做風格強化修訂：在不改變事件順序的前提下，讓文氣更貼近選定寫作風格。", nil
	case "structural":
		return "請做結構修訂：允許調整段落順序、鋪陳與揭露節奏，讓場景張力更完整。", nil
	case "sensory":
		return "", nil
	default:
		return "", fmt.Errorf("未知的修稿模式：%s", mode)
	}
}

func (s *Server) handleRewriteStream(c *gin.Context) {
	var req rewriteRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}

	instruction := ""
	if req.Mode != "sensory" {
		var err error
		instruction, err = rewriteInstruction(req.Mode)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	stylesReq := checkRequest{Checks: []string{"style"}, Styles: req.Styles}
	styles, err := s.resolveStyles(stylesReq)
	if len(req.Styles) > 0 && err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithCancel(c.Request.Context())
	msgChan := make(chan streamEvent, 256)
	var transcript strings.Builder
	rewriteBias := rewriteBiasInstruction(s.rules.Get().RewriteBias)

	go func() {
		defer cancel()
		defer close(msgChan)

		activeRetrieval := summarizeRetrieval("rewrite", mergeRetrieval(s.rules.PresetFor("rewrite"), req.Retrieval))
		references, refErr := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, retrievalOptions{
			Sources:      append([]string(nil), activeRetrieval.Sources...),
			TopK:         activeRetrieval.TopK,
			Threshold:    activeRetrieval.Threshold,
			ThresholdSet: true,
		})
		cw := &chanWriter{ch: msgChan, transcript: &transcript}
		msgChan <- streamEvent{Event: "retrieval", Retrieval: gin.H{"tasks": gin.H{"rewrite": activeRetrieval}}}
		if refErr != nil {
			text := fmt.Sprintf("\n> RAG 參考載入失敗，改用基礎模式繼續：%s\n", refErr.Error())
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
		} else {
			msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
		}

		var promptParts []string
		promptParts = append(promptParts, instruction)
		promptParts = append(promptParts, rewriteBias)
		if len(styles) > 0 {
			var styleTexts []string
			for _, style := range styles {
				styleTexts = append(styleTexts, style.RawContent)
			}
			promptParts = append(promptParts, "【寫作風格參考】\n"+strings.Join(styleTexts, "\n\n"))
		}
		if len(references) > 0 {
			promptParts = append(promptParts, "【補充參考資料】\n"+joinProfiles(references))
		}
		promptParts = append(promptParts, "【原始章節】\n"+req.Chapter)

		var rewriteErr error
		if req.Mode == "sensory" {
			rewriteErr = s.checker.EnhanceSensoryStream(ctx, strings.Join(promptParts, "\n\n"), cw)
		} else {
			rewriteErr = s.checker.RewriteChapterStream(ctx, strings.Join(promptParts, "\n\n"), cw)
		}
		if rewriteErr != nil {
			if ctx.Err() == nil {
				text := fmt.Sprintf("\n> 錯誤：%s\n", rewriteErr.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
			return
		}

		done := "\n\n---\n修稿完成\n"
		msgChan <- streamEvent{Event: "chunk", Text: done}
		transcript.WriteString(done)

		title := strings.TrimSpace(req.ChapterTitle)
		if title == "" && req.ChapterFile != "" {
			title = strings.TrimSuffix(req.ChapterFile, ".md")
		}
		if title == "" {
			title = "未命名章節"
		}
		s.history.Add(&reviewhistory.Entry{
			Kind:         "rewrite",
			ChapterTitle: title,
			ChapterFile:  strings.TrimSpace(req.ChapterFile),
			SceneTitle:   strings.TrimSpace(req.SceneTitle),
			RewriteMode:  req.Mode,
			InputContent: req.Chapter,
			Styles:       append([]string(nil), req.Styles...),
			Sources:      referenceNames(references),
			RetrievalConfigs: map[string]reviewhistory.RetrievalConfig{
				"rewrite": historyRetrievalConfig(activeRetrieval),
			},
			Result: transcript.String(),
		})
		if err := s.history.Save(); err != nil {
			log.Printf("save rewrite history: %v", err)
		}
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return false
			}
			if msg.Event == "sources" {
				c.SSEvent("sources", gin.H{"items": msg.Sources})
				return true
			}
			if msg.Event == "retrieval" {
				c.SSEvent("retrieval", msg.Retrieval)
				return true
			}
			c.SSEvent("chunk", gin.H{"text": msg.Text})
			return true
		case <-ctx.Done():
			return false
		}
	})
}

func (s *Server) handleChatStream(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "請求格式錯誤")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		c.String(http.StatusBadRequest, "訊息不可為空")
		return
	}
	character := s.profiles.FindByName(req.CharacterName)
	if character == nil {
		c.String(http.StatusBadRequest, "找不到角色："+req.CharacterName)
		return
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Content-Type-Options", "nosniff")
	flusher, _ := c.Writer.(http.Flusher)
	if err := s.checker.ChatWithCharacterStream(
		c.Request.Context(), character.RawContent, req.History, req.Message, &flushWriter{w: c.Writer, flusher: flusher},
	); err != nil {
		log.Printf("chat stream: %v", err)
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// ─── relationships ────────────────────────────────────────────────────────────

func (s *Server) handleAddRelationship(c *gin.Context) {
	r := &tracker.Relationship{
		From:         c.PostForm("from"),
		To:           c.PostForm("to"),
		Status:       c.PostForm("status"),
		Note:         c.PostForm("note"),
		TriggerEvent: c.PostForm("trigger_event"),
	}
	s.relationships.Upsert(r)
	if !saveOrAbort(c, s.relationships.Save(), "save relationships") {
		return
	}
	c.Redirect(http.StatusFound, "/relationships")
}

func (s *Server) handleDeleteRelationship(c *gin.Context) {
	s.relationships.Delete(c.PostForm("from"), c.PostForm("to"))
	if !saveOrAbort(c, s.relationships.Save(), "save relationships") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── timeline ─────────────────────────────────────────────────────────────────

func (s *Server) handleAddTimelineEvent(c *gin.Context) {
	chapter, err := parsePositiveChapter(c.PostForm("chapter"))
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	var chars []string
	for _, ch := range strings.Split(c.PostForm("characters"), ",") {
		if t := strings.TrimSpace(ch); t != "" {
			chars = append(chars, t)
		}
	}
	s.timeline.Add(&tracker.TimelineEvent{
		Chapter:      chapter,
		Scene:        c.PostForm("scene"),
		Description:  c.PostForm("description"),
		Characters:   chars,
		Consequences: c.PostForm("consequences"),
	})
	if !saveOrAbort(c, s.timeline.Save(), "save timeline") {
		return
	}
	c.Redirect(http.StatusFound, "/timeline")
}

func (s *Server) handleDeleteTimelineEvent(c *gin.Context) {
	s.timeline.Delete(c.PostForm("id"))
	if !saveOrAbort(c, s.timeline.Save(), "save timeline") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── foreshadow ───────────────────────────────────────────────────────────────

func (s *Server) handleAddForeshadow(c *gin.Context) {
	chapter, err := parsePositiveChapter(c.PostForm("chapter"))
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	s.foreshadow.Add(&tracker.Foreshadowing{
		Chapter:     chapter,
		Description: c.PostForm("description"),
		PlantedIn:   c.PostForm("planted_in"),
	})
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.Redirect(http.StatusFound, "/foreshadow")
}

func (s *Server) handleResolveForeshadow(c *gin.Context) {
	s.foreshadow.Resolve(c.PostForm("id"), c.PostForm("resolved_in"))
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.Redirect(http.StatusFound, "/foreshadow")
}

func (s *Server) handleDeleteForeshadow(c *gin.Context) {
	s.foreshadow.Delete(c.PostForm("id"))
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── export ───────────────────────────────────────────────────────────────────

func (s *Server) handleExport(c *gin.Context) {
	path, err := exporter.ExportMarkdown(
		c.PostForm("title"),
		c.PostForm("chapter"),
		c.PostForm("content"),
		s.cfg.DataDir+"/exports",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.FileAttachment(path, fmt.Sprintf("report_%s.md", c.PostForm("chapter")))
}
