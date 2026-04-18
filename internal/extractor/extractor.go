package extractor

import (
	"regexp"
	"sort"
	"strings"
)

type Signals struct {
	KnownCharacters     []string `json:"known_characters"`
	CharacterCandidates []string `json:"character_candidates"`
	LocationCandidates  []string `json:"location_candidates"`
	SettingCandidates   []string `json:"setting_candidates"`
}

var (
	hanTokenPattern     = regexp.MustCompile(`[\p{Han}]{2,4}`)
	locationPattern     = regexp.MustCompile(`(?:在|到|往|回到|走進|進入|離開)([\p{Han}]{2,8})`)
	settingLinePattern  = regexp.MustCompile(`[\p{Han}]{2,12}(規則|能力|禁忌|儀式|組織|勢力|契約|結界|血脈)`)
	characterCuePattern = regexp.MustCompile(`([\p{Han}]{2,4})(?:說|問|看見|看向|站在|走近|轉身|回答|低聲)`)
	stopwordCandidates  = map[string]struct{}{"自己": {}, "沒有": {}, "不是": {}, "看著": {}, "知道": {}, "可以": {}, "一個": {}, "兩個": {}, "如果": {}, "因為": {}, "但是": {}, "所以": {}, "時候": {}, "這裡": {}, "那裡": {}, "他們": {}, "我們": {}, "你們": {}, "突然": {}, "然而": {}, "只是": {}}
)

func AnalyzeChapter(text string, knownNames []string) Signals {
	signals := Signals{
		KnownCharacters: uniqueInText(text, knownNames),
	}

	knownLookup := make(map[string]struct{}, len(knownNames))
	for _, name := range knownNames {
		knownLookup[name] = struct{}{}
	}

	counts := make(map[string]int)
	for _, token := range hanTokenPattern.FindAllString(text, -1) {
		if _, ok := stopwordCandidates[token]; ok {
			continue
		}
		if _, ok := knownLookup[token]; ok {
			continue
		}
		counts[token]++
	}

	type scored struct {
		token string
		count int
	}
	scoredTokens := make([]scored, 0, len(counts))
	for token, count := range counts {
		if count < 2 {
			continue
		}
		scoredTokens = append(scoredTokens, scored{token: token, count: count})
	}
	sort.Slice(scoredTokens, func(i, j int) bool {
		if scoredTokens[i].count == scoredTokens[j].count {
			return scoredTokens[i].token < scoredTokens[j].token
		}
		return scoredTokens[i].count > scoredTokens[j].count
	})

	for _, item := range scoredTokens {
		if len(signals.CharacterCandidates) == 6 {
			break
		}
		signals.CharacterCandidates = append(signals.CharacterCandidates, item.token)
	}
	for _, item := range uniqueMatches(characterCuePattern, text, 1, 6) {
		if _, ok := knownLookup[item]; ok {
			continue
		}
		if contains(signals.CharacterCandidates, item) {
			continue
		}
		if len(signals.CharacterCandidates) == 6 {
			break
		}
		signals.CharacterCandidates = append(signals.CharacterCandidates, item)
	}

	signals.LocationCandidates = uniqueMatches(locationPattern, text, 1, 6)
	signals.SettingCandidates = uniqueMatches(settingLinePattern, text, 0, 6)
	return signals
}

func uniqueInText(text string, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" && strings.Contains(text, name) {
			out = append(out, name)
		}
	}
	return out
}

func uniqueMatches(pattern *regexp.Regexp, text string, index int, limit int) []string {
	matches := pattern.FindAllStringSubmatch(text, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if index >= len(match) {
			continue
		}
		item := strings.TrimSpace(match[index])
		item = strings.Trim(item, "，。、！？；：「」『』（）() ")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
