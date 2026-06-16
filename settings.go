package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are user preferences persisted to ~/.config/todoui/settings.json.
type Settings struct {
	OngoingLabel  string `json:"ongoing_label"`  // label the `o` view filters on (no @)
	FollowupLabel string `json:"followup_label"` // label the `f` view filters on (no @)
	UpNextLabel   string `json:"upnext_label"`   // label the `u` view filters on (no @)
	SyncSeconds   int    `json:"sync_seconds"`   // background auto-sync interval; 0 = off
	Light         bool   `json:"light"`          // light theme (false = dark, the default)
	DateFormat    string `json:"date_format"`    // MDY (default), YMD, or DMY
}

func defaultSettings() Settings {
	return Settings{OngoingLabel: "ongoing", FollowupLabel: "ffup", UpNextLabel: "upnext", SyncSeconds: 300, DateFormat: "MDY"}
}

func settingsPath() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "settings.json")
}

// SettingsExist reports whether the settings file has been written (first-run check).
func SettingsExist() bool {
	p := settingsPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// LoadSettings reads settings, falling back to defaults for missing/invalid fields.
func LoadSettings() Settings {
	s := defaultSettings()
	p := settingsPath()
	if p == "" {
		return s
	}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.OngoingLabel == "" {
		s.OngoingLabel = "ongoing"
	}
	if s.FollowupLabel == "" {
		s.FollowupLabel = "ffup"
	}
	if s.UpNextLabel == "" {
		s.UpNextLabel = "upnext"
	}
	if s.SyncSeconds < 0 {
		s.SyncSeconds = 0
	}
	if s.DateFormat != "YMD" && s.DateFormat != "DMY" {
		s.DateFormat = "MDY"
	}
	return s
}

// Save persists the settings.
func (s Settings) Save() {
	if p := settingsPath(); p != "" {
		if b, err := json.Marshal(s); err == nil {
			_ = os.WriteFile(p, b, 0o600)
		}
	}
}
