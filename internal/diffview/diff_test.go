package diffview

import "testing"

func TestLineDiffMarksInsertAndDelete(t *testing.T) {
	t.Parallel()

	segments := LineDiff("a\nb\nc", "a\nx\nc")
	if len(segments) < 3 {
		t.Fatalf("expected multiple diff segments, got %#v", segments)
	}
	if segments[1].Type != "delete" || segments[2].Type != "insert" {
		t.Fatalf("unexpected diff sequence: %#v", segments)
	}
}
