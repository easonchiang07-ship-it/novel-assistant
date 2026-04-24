package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/consistency"
	"novel-assistant/internal/exporter"
	"novel-assistant/internal/extractor"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/worldstate"

	"github.com/gin-gonic/gin"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type streamEvent struct {
	Event     string
	Text      string
	Layer     string
	Label     string
	Sources   []referenceSummary
	Retrieval any
	Gaps      *retrievalGaps
	Conflicts []consistency.Conflict
}

type referenceSummary struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Excerpt      string  `json:"excerpt"`
	Score        float64 `json:"score"`
	MatchReason  string  `json:"match_reason"`
	ChapterMatch string  `json:"chapter_match"`
	ChapterFile  string  `json:"chapter_file,omitempty"`
	ChapterIndex int     `json:"chapter_index,omitempty"`
	SceneIndex   int     `json:"scene_index,omitempty"`
	ChunkType    string  `json:"chunk_type,omitempty"`
}

type retrievalSummary struct {
	Task          string   `json:"task"`
	Sources       []string `json:"sources"`
	TopK          int      `json:"top_k"`
	Threshold     float64  `json:"threshold"`
	BeforeChapter int      `json:"before_chapter,omitempty"`
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

func normalizedLayerMode(raw string) string {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return "single"
	}
	return mode
}

func resolveReviewChapterMeta(req checkRequest) (chapterTitle, chapterFile string) {
	chapterFile = strings.TrimSpace(req.ChapterFile)
	chapterTitle = strings.TrimSpace(req.ChapterTitle)
	if chapterTitle == "" && chapterFile != "" {
		chapterTitle = strings.TrimSuffix(chapterFile, ".md")
	}
	if chapterTitle == "" {
		chapterTitle = "未命名章節"
	}
	return
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

func stylePresetInstruction(preset string) (string, error) {
	switch preset {
	case "":
		return "", nil
	case "cold_hard":
		return "【風格約束：冷硬派】\n- 句子保持短促，避免超過 20 字的長句\n- 以白描為主，盡量不使用形容詞疊加\n- 情緒不直接描寫，透過動作與對白呈現", nil
	case "light_novel":
		return "【風格約束：輕小說感】\n- 允許更活潑的對話節奏與內心獨白\n- 保持節奏輕快，句式清楚易讀\n- 情緒轉折可以更鮮明，但不要脫離原事件順序", nil
	case "epic":
		return "【風格約束：史詩劇感】\n- 允許較長的鋪陳句與層次分明的段落推進\n- 加強場景規模感、感官描寫與情緒重量\n- 讓敘事有莊嚴感，但不要改動核心情節", nil
	default:
		return "", fmt.Errorf("未知的風格預設：%s", preset)
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
	ChapterFile  string
	ChapterIndex int
	SceneIndex   int
	ChunkType    string
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
	if override.BeforeChapter > 0 {
		result.BeforeChapter = override.BeforeChapter
	}
	return result
}

func summarizeRetrieval(task string, opts retrievalOptions, beforeChapter int) retrievalSummary {
	return retrievalSummary{
		Task:          task,
		Sources:       append([]string(nil), opts.Sources...),
		TopK:          opts.TopK,
		Threshold:     opts.Threshold,
		BeforeChapter: beforeChapter,
	}
}

func buildConsistencyWorldContext(references []vectorProfile) string {
	if len(references) == 0 {
		return ""
	}
	var lines []string
	for _, ref := range references {
		line := fmt.Sprintf("[%s] %s：%s", ref.Type, ref.Name, excerptText(ref.Content))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Server) runConsistencyPrecheck(ctx context.Context, chapter, worldContext string, transcript *strings.Builder, msgChan chan<- streamEvent) {
	if s.consistency == nil || strings.TrimSpace(worldContext) == "" {
		return
	}
	conflicts, err := s.consistency.Check(ctx, chapter, worldContext)
	if err != nil {
		text := fmt.Sprintf("\n> 一致性預檢失敗，改為直接生成：%s\n", err.Error())
		if transcript != nil {
			transcript.WriteString(text)
		}
		msgChan <- streamEvent{Event: "chunk", Text: text}
		return
	}
	if len(conflicts) > 0 {
		msgChan <- streamEvent{Event: "conflict", Conflicts: conflicts}
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
			ChapterFile:  item.ChapterFile,
			ChapterIndex: item.ChapterIndex,
			SceneIndex:   item.SceneIndex,
			ChunkType:    item.ChunkType,
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

// resolveBeforeChapter returns the chapter index upper bound for timeline-bounded retrieval.
// If opts.BeforeChapter is set explicitly it takes precedence; otherwise the index is derived
// from chapterFile so that only chapters written before the current one are retrieved.
func resolveBeforeChapter(chapterFile string, opts retrievalOptions) int {
	if opts.BeforeChapter > 0 {
		return opts.BeforeChapter
	}
	return extractChapterIndex(chapterFile)
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

	beforeChapter := resolveBeforeChapter(chapterFile, opts)
	docs := s.store.QueryFilteredBeforeChapter(queryVec, topK, sources, threshold, beforeChapter)
	results := make([]vectorProfile, 0, len(docs))
	for _, doc := range docs {
		if doc.Type == "chapter" && strings.TrimSpace(chapterFile) != "" && doc.ChapterFile == chapterFile {
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
			ChapterFile:  doc.ChapterFile,
			ChapterIndex: doc.ChapterIndex,
			SceneIndex:   doc.SceneIndex,
			ChunkType:    doc.ChunkType,
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
		names = mergeCharacterNames(names, pronounCharacterCandidates(req.Chapter, s.profiles.Characters)...)
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

func mergeCharacterNames(base []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, name := range append(append([]string(nil), base...), extra...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, name)
	}
	return merged
}

func pronounCharacterCandidates(chapter string, chars []*profile.Character) []string {
	chapter = strings.TrimSpace(chapter)
	if chapter == "" || len(chars) == 0 {
		return nil
	}
	wantsHe := strings.Contains(chapter, "他")
	wantsShe := strings.Contains(chapter, "她")
	if !wantsHe && !wantsShe {
		return nil
	}

	out := make([]string, 0)
	for _, char := range chars {
		if char == nil || strings.TrimSpace(char.Name) == "" {
			continue
		}
		pronouns := inferCharacterPronouns(char)
		if wantsHe && contains(pronouns, "他") {
			out = append(out, char.Name)
			continue
		}
		if wantsShe && contains(pronouns, "她") {
			out = append(out, char.Name)
		}
	}
	return out
}

func inferCharacterPronouns(char *profile.Character) []string {
	if char == nil {
		return nil
	}
	text := strings.TrimSpace(char.RawContent)
	if text == "" {
		return nil
	}
	// Explicit gender keywords take priority. Pronoun occurrence in profile text
	// is only used as a fallback when no explicit keyword is found, to avoid false
	// positives when the profile mentions another character's pronouns.
	hasHeKeyword := strings.Contains(text, "男性") || strings.Contains(text, "男生") || strings.Contains(text, "男孩") || strings.Contains(text, "少年")
	hasSheKeyword := strings.Contains(text, "女性") || strings.Contains(text, "女生") || strings.Contains(text, "女孩") || strings.Contains(text, "少女")
	hasHe := hasHeKeyword || (!hasSheKeyword && strings.Contains(text, "他"))
	hasShe := hasSheKeyword || (!hasHeKeyword && strings.Contains(text, "她"))

	out := make([]string, 0, 2)
	if hasHe {
		out = append(out, "他")
	}
	if hasShe {
		out = append(out, "她")
	}
	return out
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

func (s *Server) handleAnalyzeStyle(c *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請求格式錯誤"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分析文字不可為空"})
		return
	}

	analysis, err := s.checker.AnalyzeStyle(c.Request.Context(), req.Text)
	if err != nil {
		if errors.Is(err, checker.ErrStyleParseFailure) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"analysis": analysis})
}

func (s *Server) handleApplyStyleAnalysis(c *gin.Context) {
	styleName := c.Param("name")
	style := s.profiles.FindStyleByName(styleName)
	if style == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到寫作風格：" + styleName})
		return
	}

	var analysis profile.StyleAnalysis
	if err := c.ShouldBindJSON(&analysis); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請求格式錯誤"})
		return
	}
	if err := s.saveStyleAnalysis(style.FilePath, analysis); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.profiles.Load(); err != nil {
		log.Printf("reload profiles after style analysis save: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "已套用到風格設定", "style": styleName})
}

func (s *Server) saveStyleAnalysis(styleFilePath string, analysis profile.StyleAnalysis) error {
	if strings.TrimSpace(styleFilePath) == "" {
		return fmt.Errorf("找不到風格設定檔路徑")
	}
	analysisPath := filepath.Join(filepath.Dir(styleFilePath), ".analysis", strings.TrimSuffix(filepath.Base(styleFilePath), filepath.Ext(styleFilePath))+".json")
	if err := os.MkdirAll(filepath.Dir(analysisPath), 0755); err != nil {
		return fmt.Errorf("建立風格分析目錄失敗：%w", err)
	}
	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Errorf("編碼風格分析失敗：%w", err)
	}
	if err := os.WriteFile(analysisPath, data, 0644); err != nil {
		return fmt.Errorf("寫入風格分析失敗：%w", err)
	}
	return nil
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

func (s *Server) handleDiagnosticsPage(c *gin.Context) {
	lastReindex, traces, latencyRows := s.diagnostics.snapshot()
	vectorCounts := map[string]int{}
	totalVectors := 0
	if s.store != nil {
		vectorCounts = s.store.CountsByType()
		totalVectors = s.store.Len()
	}

	c.HTML(http.StatusOK, "diagnostics.html", gin.H{
		"Title":            "Retrieval 診斷",
		"VectorCount":      totalVectors,
		"CharacterVectors": vectorCounts["character"],
		"WorldVectors":     vectorCounts["world"],
		"StyleVectors":     vectorCounts["style"],
		"ChapterVectors":   vectorCounts["chapter"],
		"LastReindex":      lastReindex,
		"RecentTraces":     traces,
		"LatencyRows":      latencyRows,
		"IndexReady":       totalVectors > 0,
	})
}

func (s *Server) handleForeshadowPage(c *gin.Context) {
	pending := s.foreshadow.GetPending()
	c.HTML(http.StatusOK, "foreshadow.html", gin.H{
		"Title":       "伏筆追蹤",
		"Foreshadows": s.foreshadow.GetAll(),
		"Pending":     pending,
	})
}

// ─── ingest ───────────────────────────────────────────────────────────────────

func (s *Server) handleIngest(c *gin.Context) {
	ctx := c.Request.Context()
	startedAt := time.Now()
	err := s.Ingest(ctx)
	finishedAt := time.Now()
	status := reindexStatus{
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
		Success:    err == nil,
		Characters: len(s.profiles.Characters),
		Worlds:     len(s.profiles.Worlds),
		Styles:     len(s.profiles.Styles),
		VectorCount: func() int {
			if s.store == nil {
				return 0
			}
			return s.store.Len()
		}(),
	}
	if chapters, listErr := s.listChapterFiles(); listErr == nil {
		status.Chapters = len(chapters)
	}
	if err != nil {
		status.Error = err.Error()
		s.diagnostics.recordReindex(status)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.diagnostics.recordReindex(status)
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
	LayerMode          string                      `json:"layer_mode"`
}

type retrievalOptions struct {
	Sources       []string `json:"sources"`
	TopK          int      `json:"top_k"`
	Threshold     float64  `json:"threshold"`
	ThresholdSet  bool     `json:"threshold_set,omitempty"`
	BeforeChapter int      `json:"before_chapter,omitempty"`
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
	layerMode := normalizedLayerMode(req.LayerMode)
	if layerMode != "single" && layerMode != "pipeline" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("未知的 layer_mode：%s", layerMode)})
		return
	}

	msgChan := make(chan streamEvent, 512)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	var transcript strings.Builder
	reviewBias := reviewBiasInstruction(s.rules.Get().ReviewBias)

	go func() {
		defer close(msgChan)

		worldStatePrefix := s.worldStateSystemPrefix(req.ChapterFile)
		if layerMode == "pipeline" {
			pipelineRefs, pipelineRetrieval, err := s.runPipelineReview(ctx, req, msgChan, &transcript, worldStatePrefix)
			if err != nil {
				return
			}

			completion := "\n\n---\n審查完成\n"
			msgChan <- streamEvent{Event: "chunk", Text: completion}
			transcript.WriteString(completion)

			chapterTitle, chapterFile := resolveReviewChapterMeta(req)
			s.history.Add(&reviewhistory.Entry{
				Kind:             "review",
				ChapterTitle:     chapterTitle,
				ChapterFile:      chapterFile,
				SceneTitle:       strings.TrimSpace(req.SceneTitle),
				InputContent:     req.Chapter,
				Checks:           append([]string(nil), req.Checks...),
				Styles:           append([]string(nil), req.Styles...),
				Sources:          referenceNames(pipelineRefs),
				RetrievalConfigs: buildHistoryRetrievalConfigs(pipelineRetrieval),
				Result:           transcript.String(),
			})
			if err := s.history.Save(); err != nil {
				log.Printf("save review history: %v", err)
			}
			return
		}

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
			activeRetrieval["behavior"] = summarizeRetrieval("behavior", behaviorOpts, resolveBeforeChapter(req.ChapterFile, behaviorOpts))
			traceStarted := time.Now()
			behaviorRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, behaviorOpts)
			s.recordRetrievalTrace("check", activeRetrieval["behavior"], req.ChapterTitle, req.ChapterFile, behaviorRefs, err, time.Since(traceStarted))
			if err != nil {
				text := fmt.Sprintf("\n> 行為審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}
		if contains(req.Checks, "dialogue") {
			dialogueOpts := mergeRetrieval(s.rules.PresetFor("dialogue"), req.retrievalOverrideFor("dialogue"))
			activeRetrieval["dialogue"] = summarizeRetrieval("dialogue", dialogueOpts, resolveBeforeChapter(req.ChapterFile, dialogueOpts))
			traceStarted := time.Now()
			dialogueRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, dialogueOpts)
			s.recordRetrievalTrace("check", activeRetrieval["dialogue"], req.ChapterTitle, req.ChapterFile, dialogueRefs, err, time.Since(traceStarted))
			if err != nil {
				text := fmt.Sprintf("\n> 對白審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}
		if contains(req.Checks, "world") {
			worldOpts := mergeRetrieval(s.rules.PresetFor("world"), req.retrievalOverrideFor("world"))
			activeRetrieval["world"] = summarizeRetrieval("world", worldOpts, resolveBeforeChapter(req.ChapterFile, worldOpts))
			traceStarted := time.Now()
			worldRefs, err = s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, worldOpts)
			s.recordRetrievalTrace("check", activeRetrieval["world"], req.ChapterTitle, req.ChapterFile, worldRefs, err, time.Since(traceStarted))
			if err != nil {
				text := fmt.Sprintf("\n> 世界觀審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
		}

		msgChan <- streamEvent{Event: "retrieval", Retrieval: gin.H{"tasks": activeRetrieval}}

		references := mergeReferenceLists(behaviorRefs, dialogueRefs, worldRefs)
		indexReady := s.store != nil && s.store.Len() > 0
		worldContext := buildConsistencyWorldContext(references)
		if len(references) > 0 {
			msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
			gaps := computeRetrievalGaps(req.Chapter, s.profiles.AllNames(), references)
			gaps.IndexReady = indexReady
			if len(gaps.MissingCharacters)+len(gaps.MissingLocations)+len(gaps.MissingSettings) > 0 {
				msgChan <- streamEvent{Event: "gaps", Gaps: &gaps}
			}
			s.runConsistencyPrecheck(ctx, req.Chapter, worldContext, &transcript, msgChan)
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
				if err := s.checker.CheckWorldConflictWithSystemStream(ctx, worldStatePrefix, worldPrompt, req.Chapter, cw); err != nil {
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
			if err := s.checker.DiagnoseOpeningWithSystemStream(ctx, worldStatePrefix, req.Chapter, cw); err != nil {
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
				if err := s.checker.CheckBehaviorWithSystemStream(ctx, worldStatePrefix, profileText, req.Chapter, cw); err != nil {
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
				if err := s.checker.CheckDialogueWithSystemStream(ctx, worldStatePrefix, char.Name, char.Personality, dialogueStyle, req.Chapter, cw); err != nil {
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
			if err := s.checker.CheckStyleWithSystemStream(ctx, worldStatePrefix, stylePrompt, req.Chapter, cw); err != nil {
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

		chapterTitle, chapterFile := resolveReviewChapterMeta(req)
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

	// Drain all events until the goroutine closes msgChan. ctx.Done() only
	// fires on genuine client disconnect (parent request context), not when
	// the goroutine finishes — cancel() is deferred in this function, so it
	// only fires after we return.
	for msg := range msgChan {
		select {
		case <-ctx.Done():
			return
		default:
		}
		switch msg.Event {
		case "sources":
			c.SSEvent("sources", gin.H{"items": msg.Sources})
		case "retrieval":
			c.SSEvent("retrieval", msg.Retrieval)
		case "gaps":
			c.SSEvent("gaps", msg.Gaps)
		case "layer_start":
			c.SSEvent("layer_start", gin.H{"layer": msg.Layer, "label": msg.Label})
		case "layer_end":
			c.SSEvent("layer_end", gin.H{"layer": msg.Layer})
		case "conflict":
			c.SSEvent("conflict", gin.H{"conflicts": msg.Conflicts})
		default:
			c.SSEvent("chunk", gin.H{"text": msg.Text})
		}
		c.Writer.Flush()
	}
}

type rewriteRequest struct {
	Chapter      string           `json:"chapter"`
	Mode         string           `json:"mode"`
	Characters   []string         `json:"characters"`
	Styles       []string         `json:"styles"`
	StylePreset  string           `json:"style_preset"`
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
	stylePresetText, err := stylePresetInstruction(strings.TrimSpace(req.StylePreset))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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

		worldStatePrefix := s.worldStateSystemPrefix(req.ChapterFile)
		rewriteOpts := mergeRetrieval(s.rules.PresetFor("rewrite"), req.Retrieval)
		activeRetrieval := summarizeRetrieval("rewrite", rewriteOpts, resolveBeforeChapter(req.ChapterFile, rewriteOpts))
		traceStarted := time.Now()
		references, refErr := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, rewriteOpts)
		s.recordRetrievalTrace("rewrite", activeRetrieval, req.ChapterTitle, req.ChapterFile, references, refErr, time.Since(traceStarted))
		cw := &chanWriter{ch: msgChan, transcript: &transcript}
		msgChan <- streamEvent{Event: "retrieval", Retrieval: gin.H{"tasks": gin.H{"rewrite": activeRetrieval}}}
		if refErr != nil {
			text := fmt.Sprintf("\n> RAG 參考載入失敗，改用基礎模式繼續：%s\n", refErr.Error())
			transcript.WriteString(text)
			msgChan <- streamEvent{Event: "chunk", Text: text}
		} else {
			msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
		}
		s.runConsistencyPrecheck(ctx, req.Chapter, buildConsistencyWorldContext(references), &transcript, msgChan)

		var promptParts []string
		promptParts = append(promptParts, instruction)
		promptParts = append(promptParts, rewriteBias)
		if stylePresetText != "" {
			promptParts = append(promptParts, stylePresetText)
		}
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
			rewriteErr = s.checker.EnhanceSensoryWithSystemStream(ctx, worldStatePrefix, strings.Join(promptParts, "\n\n"), cw)
		} else {
			rewriteErr = s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, strings.Join(promptParts, "\n\n"), cw)
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
			if msg.Event == "conflict" {
				c.SSEvent("conflict", gin.H{"conflicts": msg.Conflicts})
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

func (s *Server) handleDetectForeshadow(c *gin.Context) {
	var req struct {
		Chapter      string `json:"chapter"`
		ChapterIndex int    `json:"chapter_index"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	if len(req.Chapter) > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可超過 20000 字元"})
		return
	}
	if req.ChapterIndex < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_index 必須為正整數"})
		return
	}
	candidates, err := s.checker.ExtractHooks(c.Request.Context(), req.Chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hooks := make([]tracker.PendingHook, len(candidates))
	for i, cand := range candidates {
		hooks[i] = tracker.PendingHook{
			Description:  cand.Description,
			Context:      cand.Context,
			Confidence:   cand.Confidence,
			ChapterIndex: req.ChapterIndex,
		}
	}
	s.foreshadow.AddPending(hooks)

	// Update LastSeenChapter for any existing confirmed hook whose description
	// appears verbatim in the chapter text.
	var touchIDs []string
	for _, item := range s.foreshadow.GetAll() {
		if strings.Contains(req.Chapter, item.Description) {
			touchIDs = append(touchIDs, item.ID)
		}
	}
	s.foreshadow.TouchLastSeen(req.ChapterIndex, touchIDs)

	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"pending": s.foreshadow.GetPending()})
}

func (s *Server) handleConfirmForeshadow(c *gin.Context) {
	var req struct {
		ID        string `json:"id"`
		Chapter   int    `json:"chapter"`
		PlantedIn string `json:"planted_in"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Chapter <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter 必須為正整數"})
		return
	}
	if !s.foreshadow.ConfirmPending(req.ID, req.Chapter, req.PlantedIn) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pending hook not found"})
		return
	}
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "pending": s.foreshadow.GetPending()})
}

func (s *Server) handleDismissForeshadow(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !s.foreshadow.DismissPending(req.ID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pending hook not found"})
		return
	}
	if !saveOrAbort(c, s.foreshadow.Save(), "save foreshadow") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "pending": s.foreshadow.GetPending()})
}

func (s *Server) handleStaleForeshadow(c *gin.Context) {
	currentChapter, err := parsePositiveChapter(c.Query("current_chapter"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_chapter 必須為正整數"})
		return
	}
	threshold := 3
	if t := c.Query("threshold"); t != "" {
		v, err := parsePositiveChapter(t)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "threshold 必須為正整數"})
			return
		}
		threshold = v
	}
	stale := s.foreshadow.StaleForeshadows(currentChapter, threshold)
	if stale == nil {
		stale = []*tracker.Foreshadowing{}
	}
	c.JSON(http.StatusOK, gin.H{"items": stale})
}

// ─── narrative memory ─────────────────────────────────────────────────────────

func (s *Server) handleNarrativePage(c *gin.Context) {
	c.HTML(http.StatusOK, "narrative.html", gin.H{
		"Title": "敘事記憶抽取",
	})
}

func (s *Server) handleNarrativeExtract(c *gin.Context) {
	var req struct {
		Chapter      string `json:"chapter"`
		ChapterIndex int    `json:"chapter_index"`
		ChapterFile  string `json:"chapter_file"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	if len(req.Chapter) > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可超過 20000 字元"})
		return
	}
	if req.ChapterIndex < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_index 必須為正整數"})
		return
	}

	mem, err := s.checker.ExtractNarrativeMemory(c.Request.Context(), req.Chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, ev := range mem.Events {
		s.timeline.Add(&tracker.TimelineEvent{
			Chapter:      req.ChapterIndex,
			Scene:        ev.Scene,
			Description:  ev.Description,
			Characters:   ev.Characters,
			Consequences: ev.Consequences,
		})
	}
	if len(mem.Events) > 0 {
		if err := s.timeline.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存時間軸失敗"})
			return
		}
	}

	for _, rel := range mem.Relationships {
		s.relationships.Upsert(&tracker.Relationship{
			From:         rel.From,
			To:           rel.To,
			Status:       rel.Status,
			Note:         rel.Note,
			TriggerEvent: rel.TriggerEvent,
		})
	}
	if len(mem.Relationships) > 0 {
		if err := s.relationships.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存角色關係失敗"})
			return
		}
	}

	if len(mem.WorldState) > 0 {
		s.worldstate.Upsert(&worldstate.Snapshot{
			ChapterFile:  req.ChapterFile,
			ChapterIndex: req.ChapterIndex,
			Changes:      mem.WorldState,
		})
		if err := s.worldstate.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存世界觀狀態失敗"})
			return
		}
	}

	if len(mem.Hooks) > 0 {
		hooks := make([]tracker.PendingHook, len(mem.Hooks))
		for i, h := range mem.Hooks {
			hooks[i] = tracker.PendingHook{
				Description:  h.Description,
				Context:      h.Context,
				Confidence:   h.Confidence,
				ChapterIndex: req.ChapterIndex,
			}
		}
		s.foreshadow.AddPending(hooks)
		if err := s.foreshadow.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存伏筆失敗"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"events":        mem.Events,
		"relationships": mem.Relationships,
		"world_state":   mem.WorldState,
		"hooks":         mem.Hooks,
	})
}

// ─── evaluate ─────────────────────────────────────────────────────────────────

type evaluateRequest struct {
	Chapter        string `json:"chapter"`
	ChapterIndex   int    `json:"chapter_index"`
	ChapterFile    string `json:"chapter_file"`
	StaleThreshold int    `json:"stale_threshold"`
	CompareChapter string `json:"compare_chapter"`
}

type ScoreAdjustment struct {
	Label string `json:"label"`
	Delta int    `json:"delta"`
}

type FinalEvaluation struct {
	Stability   *checker.StabilityResult `json:"stability"`
	BaseScore   int                      `json:"base_score"`
	Adjustments []ScoreAdjustment        `json:"adjustments"`
	FinalScore  int                      `json:"final_score"`
	Compare     *checker.StabilityResult `json:"compare,omitempty"`
}

func (s *Server) computeAdjustments(ctx context.Context, chapterIndex, staleThreshold int, worldContext, chapter string) ([]ScoreAdjustment, int) {
	var adjustments []ScoreAdjustment
	total := 0

	if s.foreshadow != nil && chapterIndex > 0 {
		threshold := staleThreshold
		if threshold <= 0 {
			threshold = 10
		}
		stale := s.foreshadow.StaleForeshadows(chapterIndex, threshold)
		if len(stale) > 0 {
			adj := ScoreAdjustment{Label: fmt.Sprintf("過期伏筆 %d 條", len(stale)), Delta: -5}
			adjustments = append(adjustments, adj)
			total += adj.Delta
		}
	}

	if s.consistency != nil && chapter != "" {
		conflicts, err := s.consistency.Check(ctx, chapter, worldContext)
		if err == nil {
			cap := 3
			if len(conflicts) < cap {
				cap = len(conflicts)
			}
			for i := 0; i < cap; i++ {
				adj := ScoreAdjustment{Label: fmt.Sprintf("世界觀衝突：%s", conflicts[i].Description), Delta: -3}
				adjustments = append(adjustments, adj)
				total += adj.Delta
			}
		}
	}

	return adjustments, total
}

func (s *Server) handleEvaluatePage(c *gin.Context) {
	c.HTML(http.StatusOK, "evaluate.html", nil)
}

func (s *Server) handleEvaluateStream(c *gin.Context) {
	var req evaluateRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}

	type evalEvent struct {
		kind    string
		run     int
		payload any
	}

	msgChan := make(chan evalEvent, 16)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	worldPrefix := s.worldStateSystemPrefix(req.ChapterFile)

	go func() {
		defer close(msgChan)

		onProgress := func(run, total int) {
			msgChan <- evalEvent{kind: "progress", run: run, payload: gin.H{"run": run, "total": total}}
		}

		stability, err := s.checker.EvaluateChapter(ctx, req.Chapter, worldPrefix, onProgress)
		if err != nil {
			msgChan <- evalEvent{kind: "error", payload: gin.H{"error": err.Error()}}
			return
		}

		baseScore := checker.WeightedScore(stability.MedianScores)
		adjustments, delta := s.computeAdjustments(ctx, req.ChapterIndex, req.StaleThreshold, worldPrefix, req.Chapter)
		finalScore := baseScore + delta
		if finalScore < 0 {
			finalScore = 0
		}
		if finalScore > 100 {
			finalScore = 100
		}

		eval := &FinalEvaluation{
			Stability:   stability,
			BaseScore:   baseScore,
			Adjustments: adjustments,
			FinalScore:  finalScore,
		}

		if strings.TrimSpace(req.CompareChapter) != "" {
			compare, err := s.checker.EvaluateChapter(ctx, req.CompareChapter, worldPrefix, nil)
			if err == nil {
				eval.Compare = compare
			}
		}

		msgChan <- evalEvent{kind: "result", payload: eval}
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	for msg := range msgChan {
		select {
		case <-ctx.Done():
			return
		default:
		}
		c.SSEvent(msg.kind, msg.payload)
		c.Writer.Flush()
	}
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
