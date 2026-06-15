package main

import (
	"os"
	"path/filepath"
	"strings"
)

// stateDir returns ~/.config/todoui, creating it if needed.
func stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".config", "todoui")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func stateFile() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "last_project.txt")
}

// LoadLastProject reads the last chosen project ("ID\tName"). Empty if none.
func LoadLastProject() Project {
	f := stateFile()
	if f == "" {
		return Project{}
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return Project{}
	}
	parts := strings.SplitN(strings.TrimSpace(string(b)), "\t", 2)
	if len(parts) != 2 {
		return Project{}
	}
	return Project{ID: parts[0], Name: parts[1]}
}

// SaveLastProject persists the chosen project for next time.
func SaveLastProject(p Project) {
	f := stateFile()
	if f == "" || p.ID == "" {
		return
	}
	_ = os.WriteFile(f, []byte(p.ID+"\t"+p.Name), 0o644)
}
