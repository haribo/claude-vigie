// Package config loads the shared per-machine claude-fleet client
// configuration (server URL, auth token, machine name) used by the reporter
// and the terminal client. It lives in the XDG config directory, is written
// by `claude-fleet init`, and is never committed (it holds a secret token).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the per-machine client configuration.
type Config struct {
	// ServerURL is the base URL of the claude-fleet server (e.g. http://host:8080).
	ServerURL string `json:"server_url"`
	// Token is the shared secret sent in the Authorization header when reporting.
	Token string `json:"token"`
	// Machine is the human-readable name of this machine, used to group sessions.
	Machine string `json:"machine"`
}

// Path returns the config file path, honoring XDG_CONFIG_HOME on Linux.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(dir, "claude-fleet", "config.json"), nil
}

// Load reads and parses the config file.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes the config file with 0600 permissions (it holds a secret),
// creating the parent directory if needed. It returns the written path.
func Save(cfg *Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing config %s: %w", path, err)
	}
	return path, nil
}
