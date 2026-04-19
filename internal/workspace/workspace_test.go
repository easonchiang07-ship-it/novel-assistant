package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDataDir_Default(t *testing.T) {
	t.Parallel()

	if got := ProjectDataDir("default"); got != "data" {
		t.Fatalf("expected default data dir to be data, got %q", got)
	}
}

func TestProjectDataDir_Custom(t *testing.T) {
	t.Parallel()

	if got := ProjectDataDir("novel2"); got != filepath.Join("workspaces", "novel2") {
		t.Fatalf("expected custom data dir under workspaces, got %q", got)
	}
}

func TestEnsureIndex_NoFile_ReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	idx, err := EnsureIndex()
	if err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if idx.Active != "default" {
		t.Fatalf("expected active default, got %q", idx.Active)
	}
	if len(idx.Names) != 1 || idx.Names[0] != "default" {
		t.Fatalf("expected names [default], got %#v", idx.Names)
	}
}

func TestSaveAndLoadIndex(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	want := Index{
		Active: "novel2",
		Names:  []string{"default", "novel2"},
	}
	if err := SaveIndex(want); err != nil {
		t.Fatalf("save index: %v", err)
	}

	got, err := EnsureIndex()
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	if got.Active != want.Active {
		t.Fatalf("expected active %q, got %q", want.Active, got.Active)
	}
	if strings.Join(got.Names, ",") != strings.Join(want.Names, ",") {
		t.Fatalf("expected names %v, got %v", want.Names, got.Names)
	}
}

func TestValidateName(t *testing.T) {
	t.Parallel()

	valid := []string{"my-novel", "novel_2", "NovelA"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Fatalf("expected %q valid, got %v", name, err)
		}
	}

	invalid := []string{"", "../../etc", "a b", strings.Repeat("a", 65)}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Fatalf("expected %q invalid", name)
		}
	}
}
