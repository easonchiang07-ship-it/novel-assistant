package projectsettings

import "testing"

func TestNormalizeFillsDefaults(t *testing.T) {
	t.Parallel()

	item := normalize(Settings{})
	if item.OllamaURL == "" || item.LLMModel == "" || item.EmbedModel == "" || item.Port == "" {
		t.Fatalf("expected defaults, got %#v", item)
	}
	if item.BackupRetention < 1 {
		t.Fatalf("expected backup retention default, got %#v", item)
	}
}
