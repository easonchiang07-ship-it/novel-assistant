package server

import (
	"context"
	"fmt"
	"strings"
)

type reviewLayer struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

func defaultReviewLayers() []reviewLayer {
	return []reviewLayer{
		{
			Name:    "structure",
			Label:   "結構層",
			Prompt:  "你是專業小說結構編輯。只分析以下章節的敘事節奏、開場鉤子、張力起伏與段落長短，不要評論角色或語言風格。",
			Enabled: true,
		},
		{
			Name:    "character",
			Label:   "角色層",
			Prompt:  "你是角色塑造專家。只分析以下章節的角色行為是否符合人設、對白語氣是否一致，並結合提供的角色資料判斷，不要評論結構或語言。",
			Enabled: true,
		},
		{
			Name:    "world_logic",
			Label:   "世界觀層",
			Prompt:  "你是世界觀邏輯審查員。只分析以下章節的設定自洽性、時間線合理性、道具與地點邏輯，不要評論其他層面。",
			Enabled: true,
		},
		{
			Name:    "language",
			Label:   "語言層",
			Prompt:  "你是文字風格編輯。只分析以下章節的句式多樣性、重複用語、感官描寫密度與語言流暢度，不要評論劇情或角色。",
			Enabled: true,
		},
	}
}

func resolveReviewLayers(req checkRequest) []reviewLayer {
	if req.LayerMode != "pipeline" {
		return nil
	}
	return defaultReviewLayers()
}

func (s *Server) runPipelineReview(
	ctx context.Context,
	req checkRequest,
	msgChan chan<- streamEvent,
	transcript *strings.Builder,
	worldStatePrefix string,
) error {
	layers := resolveReviewLayers(req)
	if len(layers) == 0 {
		return nil
	}

	charsToCheck := s.resolveCharacters(req)

	reviewBias := reviewBiasInstruction(s.rules.Get().ReviewBias)
	behaviorOpts := mergeRetrieval(s.rules.PresetFor("behavior"), req.retrievalOverrideFor("behavior"))
	dialogueOpts := mergeRetrieval(s.rules.PresetFor("dialogue"), req.retrievalOverrideFor("dialogue"))
	worldOpts := mergeRetrieval(s.rules.PresetFor("world"), req.retrievalOverrideFor("world"))
	activeRetrieval := map[string]retrievalSummary{
		"behavior": summarizeRetrieval("behavior", behaviorOpts),
		"dialogue": summarizeRetrieval("dialogue", dialogueOpts),
		"world":    summarizeRetrieval("world", worldOpts),
	}

	behaviorRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, behaviorOpts)
	if err != nil {
		text := fmt.Sprintf("\n> 行為審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
		transcript.WriteString(text)
		msgChan <- streamEvent{Event: "chunk", Text: text}
		behaviorRefs = nil
	}
	dialogueRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, dialogueOpts)
	if err != nil {
		text := fmt.Sprintf("\n> 對白審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
		transcript.WriteString(text)
		msgChan <- streamEvent{Event: "chunk", Text: text}
		dialogueRefs = nil
	}
	worldRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, worldOpts)
	if err != nil {
		text := fmt.Sprintf("\n> 世界觀審查的 RAG 參考載入失敗，改用基礎模式繼續：%s\n", err.Error())
		transcript.WriteString(text)
		msgChan <- streamEvent{Event: "chunk", Text: text}
		worldRefs = nil
	}

	msgChan <- streamEvent{Event: "retrieval", Retrieval: map[string]any{"tasks": activeRetrieval}}

	references := mergeReferenceLists(behaviorRefs, dialogueRefs, worldRefs)
	if len(references) > 0 {
		msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
	}
	if len(charsToCheck) > 0 {
		gaps := computeRetrievalGaps(req.Chapter, s.profiles.AllNames(), references)
		if s.store.Len() > 0 {
			gaps.IndexReady = true
		}
		if len(gaps.MissingCharacters) > 0 || len(gaps.MissingLocations) > 0 || len(gaps.MissingSettings) > 0 {
			msgChan <- streamEvent{Event: "gaps", Gaps: &gaps}
		}
	}

	worldText := joinWorldProfiles(filterReferencesByType(worldRefs, "world"), s.profiles.Worlds)

	styleReq := req
	styleReq.Checks = []string{"style"}
	stylesToCheck, err := s.resolveStyles(styleReq)
	if len(req.Styles) > 0 && err != nil {
		text := fmt.Sprintf("\n> 錯誤：%s\n", err.Error())
		transcript.WriteString(text)
		msgChan <- streamEvent{Event: "chunk", Text: text}
		return err
	}

	styleTexts := make([]string, 0, len(stylesToCheck))
	for _, sg := range stylesToCheck {
		styleTexts = append(styleTexts, sg.RawContent)
	}
	languageRefs := joinProfiles(filterReferencesByType(references, "style"))
	if len(styleTexts) > 0 {
		if languageRefs != "" {
			languageRefs += "\n\n"
		}
		languageRefs += strings.Join(styleTexts, "\n\n")
	}

	cw := &chanWriter{ch: msgChan, transcript: transcript}
	for _, layer := range layers {
		msgChan <- streamEvent{Event: "layer_start", Layer: layer.Name, Label: layer.Label}

		var promptParts []string
		promptParts = append(promptParts, layer.Prompt)
		promptParts = append(promptParts, "【審查偏好】\n"+reviewBias)

		switch layer.Name {
		case "character":
			profiles := make([]string, 0, len(charsToCheck))
			for _, ch := range charsToCheck {
				profiles = append(profiles, ch.RawContent)
			}
			if len(profiles) > 0 {
				promptParts = append(promptParts, "【角色資料】\n"+strings.Join(profiles, "\n\n"))
			}
			layerRefs := joinProfiles(mergeReferenceLists(behaviorRefs, dialogueRefs))
			if layerRefs != "" {
				promptParts = append(promptParts, "【補充參考資料】\n"+layerRefs)
			}
		case "world_logic":
			if strings.TrimSpace(worldText) != "" {
				promptParts = append(promptParts, "【世界觀資料】\n"+worldText)
			}
		case "language":
			if strings.TrimSpace(languageRefs) != "" {
				promptParts = append(promptParts, "【補充參考資料】\n"+languageRefs)
			}
		}

		promptParts = append(promptParts, "【章節內容】\n"+req.Chapter)
		layerErr := s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, strings.Join(promptParts, "\n\n"), cw)
		if layerErr != nil {
			if ctx.Err() == nil {
				text := fmt.Sprintf("\n> 錯誤：%s\n", layerErr.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
			return layerErr
		}

		msgChan <- streamEvent{Event: "layer_end", Layer: layer.Name}
		msgChan <- streamEvent{Event: "chunk", Text: "\n"}
		transcript.WriteString("\n")
	}

	return nil
}
