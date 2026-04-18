package server

import "testing"

func TestDraftTargetPathRejectsInvalidName(t *testing.T) {
	t.Parallel()

	if _, _, err := draftTargetPath("data", "character", "../oops"); err == nil {
		t.Fatal("expected invalid draft path to be rejected")
	}
}

func TestDraftTargetPathSupportsCharacterAndWorld(t *testing.T) {
	t.Parallel()

	path, file, err := draftTargetPath("data", "character", "沈墨")
	if err != nil || file != "沈墨.md" || path == "" {
		t.Fatalf("unexpected character draft target: path=%q file=%q err=%v", path, file, err)
	}

	path, file, err = draftTargetPath("data", "world", "夜港塔")
	if err != nil || file != "夜港塔.md" || path == "" {
		t.Fatalf("unexpected world draft target: path=%q file=%q err=%v", path, file, err)
	}
}
