package setup

import (
	"os"
	"path/filepath"
)

const setupDoneMarker = ".setup_complete"

// IsComplete returns true if the one-time setup wizard has been finished.
func IsComplete(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, setupDoneMarker))
	return err == nil
}

// MarkComplete writes the marker file so future launches skip the wizard.
func MarkComplete(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, setupDoneMarker), []byte("1"), 0o644)
}
