package extractor

import "testing"

func TestAnalyzeChapterFindsKnownCharactersAndCandidates(t *testing.T) {
	t.Parallel()

	signals := AnalyzeChapter("林昊走進夜港塔，夜港塔的鐘聲讓黑潮會的人都停下。林昊看見沈墨正站在塔下。", []string{"林昊"})
	if len(signals.KnownCharacters) != 1 || signals.KnownCharacters[0] != "林昊" {
		t.Fatalf("unexpected known characters: %#v", signals.KnownCharacters)
	}
	if len(signals.LocationCandidates) == 0 {
		t.Fatalf("expected location candidates, got %#v", signals.LocationCandidates)
	}
	if len(signals.CharacterCandidates) == 0 {
		t.Fatalf("expected character candidates, got %#v", signals.CharacterCandidates)
	}
}

func TestAnalyzeChapterFindsSettingCandidates(t *testing.T) {
	t.Parallel()

	signals := AnalyzeChapter("血契規則一旦成立，結界規則就會同步收緊。", nil)
	if len(signals.SettingCandidates) == 0 {
		t.Fatalf("expected setting candidates, got %#v", signals.SettingCandidates)
	}
}
