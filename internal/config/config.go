// Package config holds the on-disk preferences (config.yaml) and window
// state (state.json) shared between the countdown CLI and any companion
// tooling (e.g. a GUI) that wants to read or write the same files.
package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ValidCorners lists the accepted values for the -corner flag/config field.
var ValidCorners = []string{"top-left", "top-right", "bottom-left", "bottom-right"}

// WindowState is persisted across runs so the overlay reopens where it was left.
type WindowState struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	MonitorIdx int `json:"monitorIdx"`
}

// StatePath returns ~/.config/countdown/state.json (or the OS equivalent).
func StatePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "countdown", "state.json"), nil
}

// LoadWindowState reads the persisted window state, if any.
func LoadWindowState() (*WindowState, bool) {
	path, err := StatePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var state WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false
	}
	return &state, true
}

// SaveWindowState persists the window's position and monitor.
func SaveWindowState(x, y, monitorIdx int) error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(WindowState{X: x, Y: y, MonitorIdx: monitorIdx})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Config holds user preferences loaded from a YAML config file. Fields are
// pointers so callers can tell "not set in the file" apart from a
// legitimate zero value, which matters for merging with CLI flags and
// built-in defaults.
type Config struct {
	Corner  *string  `yaml:"corner,omitempty"`
	Monitor *int     `yaml:"monitor,omitempty"`
	Timeout *int     `yaml:"timeout,omitempty"`
	Scale   *float64 `yaml:"scale,omitempty"`
	Padding *int     `yaml:"padding,omitempty"`
	Hours   *int     `yaml:"hours,omitempty"`
	Minutes *int     `yaml:"minutes,omitempty"`
	Seconds *int     `yaml:"seconds,omitempty"`
}

// DefaultPath returns ~/.config/countdown/config.yaml (or the OS equivalent).
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "countdown", "config.yaml"), nil
}

// Load reads the config file at path. A missing file is not an error (it
// just means no preferences are set); a malformed one is fatal, since
// silently ignoring a typo'd config would be confusing.
func Load(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}
		}
		log.Fatalf("Failed to read config file %s: %v", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config file %s: %v", path, err)
	}
	return cfg
}

// Write marshals cfg to YAML and writes it to path, creating the parent
// directory if needed.
func Write(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
