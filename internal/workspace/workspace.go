package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)

const indexPath = "workspaces/index.json"

type Index struct {
	Active string   `json:"active"`
	Names  []string `json:"names"`
}

func ProjectDataDir(name string) string {
	if name == "default" {
		return "data"
	}
	return filepath.Join("workspaces", name)
}

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("專案名稱只能包含英數字、-、_，長度 1-64")
	}
	return nil
}

func EnsureIndex() (Index, error) {
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return Index{Active: "default", Names: []string{"default"}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	if idx.Active == "" {
		idx.Active = "default"
	}
	if len(idx.Names) == 0 {
		idx.Names = []string{"default"}
	}
	return idx, nil
}

func SaveIndex(idx Index) error {
	if err := os.MkdirAll("workspaces", 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

func ContainsName(idx Index, name string) bool {
	for _, n := range idx.Names {
		if n == name {
			return true
		}
	}
	return false
}
