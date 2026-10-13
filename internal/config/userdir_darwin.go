package config

import (
	"errors"
	"os"
	"path/filepath"
)

// UserDir is the base directory for per-user config. On macOS it follows the XDG
// rule rather than os.UserConfigDir's ~/Library/Application Support (ADR-0018):
// the paths the docs give are then true on both platforms, and XDG_CONFIG_HOME
// isolates a test run here as it does on Linux.
func UserDir() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		if !filepath.IsAbs(d) {
			return "", errors.New("path in $XDG_CONFIG_HOME is relative")
		}
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}
