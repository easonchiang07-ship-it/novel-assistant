package server

import "testing"

func TestProjectTemplateFilesReturnsUrbanFantasyStarterFiles(t *testing.T) {
	t.Parallel()

	files, err := projectTemplateFiles("urban-fantasy")
	if err != nil {
		t.Fatalf("expected template files, got error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected non-empty template files")
	}
}

func TestProjectTemplateFilesRejectsUnknownTemplate(t *testing.T) {
	t.Parallel()

	if _, err := projectTemplateFiles("unknown"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}
