package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newWritebackTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(ollama.Close)
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", ".keep"), "")
	s := newE2ETestServer(t, dir, ollama.URL)
	srv := httptest.NewServer(s.router)
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func TestWritebackTimelineStoresSceneIndex(t *testing.T) {
	t.Parallel()
	s, baseURL := newWritebackTestServer(t)

	resp := performJSONRequest(t, baseURL, "POST", "/api/writeback/timeline", map[string]any{
		"chapter":      2,
		"scene_index":  3,
		"scene":        "廢棄站台",
		"description":  "林昊發現線索",
		"characters":   []string{"林昊"},
		"consequences": "調查深入",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	events := s.timeline.GetSorted()
	if len(events) != 1 {
		t.Fatalf("expected 1 timeline event, got %d", len(events))
	}
	if events[0].SceneIndex != 3 {
		t.Errorf("SceneIndex: got %d, want 3", events[0].SceneIndex)
	}
	if events[0].Chapter != 2 {
		t.Errorf("Chapter: got %d, want 2", events[0].Chapter)
	}
}

func TestWritebackTimelineZeroSceneIndexWhenOmitted(t *testing.T) {
	t.Parallel()
	s, baseURL := newWritebackTestServer(t)

	resp := performJSONRequest(t, baseURL, "POST", "/api/writeback/timeline", map[string]any{
		"chapter":     1,
		"scene":       "序章",
		"description": "故事開始",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	events := s.timeline.GetSorted()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SceneIndex != 0 {
		t.Errorf("SceneIndex: got %d, want 0 when omitted", events[0].SceneIndex)
	}
}

func TestWritebackForeshadowStoresSceneIndex(t *testing.T) {
	t.Parallel()
	s, baseURL := newWritebackTestServer(t)

	resp := performJSONRequest(t, baseURL, "POST", "/api/writeback/foreshadow", map[string]any{
		"chapter":     1,
		"scene_index": 2,
		"description": "神秘信件",
		"planted_in":  "林昊看了一眼桌上的信，沒有打開",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	items := s.foreshadow.GetAll()
	if len(items) != 1 {
		t.Fatalf("expected 1 foreshadow, got %d", len(items))
	}
	if items[0].SceneIndex != 2 {
		t.Errorf("SceneIndex: got %d, want 2", items[0].SceneIndex)
	}
}

func TestWritebackRelationshipStoresChapterAndSceneIndex(t *testing.T) {
	t.Parallel()
	s, baseURL := newWritebackTestServer(t)

	resp := performJSONRequest(t, baseURL, "POST", "/api/writeback/relationship", map[string]any{
		"from":          "林昊",
		"to":            "陳晨",
		"status":        "盟友",
		"note":          "共同調查",
		"trigger_event": "廢棄站台會面",
		"chapter":       3,
		"scene_index":   1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	rels := s.relationships.GetAll()
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}
	if rels[0].Chapter != 3 {
		t.Errorf("Chapter: got %d, want 3", rels[0].Chapter)
	}
	if rels[0].SceneIndex != 1 {
		t.Errorf("SceneIndex: got %d, want 1", rels[0].SceneIndex)
	}
}
