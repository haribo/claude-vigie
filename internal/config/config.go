// Package config loads the shared per-machine vigie client
// configuration (server URL, auth token, machine name) used by the reporter
// and the terminal client. It lives in the XDG config directory as a TOML file
// (config.toml), is written by `vigie init`, and is never committed (it
// holds a secret token). A pre-TOML config.json is migrated on first load.
//
// VIGIE_CONFIG (or the deprecated FLEET_CONFIG) overrides the path with an
// explicit file, so a dev run can point at a local server without touching the
// installed production config.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is the per-machine client configuration. Both toml and json tags are
// declared so the legacy config.json can be migrated (see migrateLegacy).
type Config struct {
	// ServerURL is the base URL of the vigied server (e.g. http://host:8080).
	ServerURL string `toml:"server_url" json:"server_url"`
	// Token is the shared secret sent in the Authorization header when reporting.
	Token string `toml:"token" json:"token"`
	// Machine is the human-readable name of this machine, used to group sessions.
	Machine string `toml:"machine" json:"machine"`
}

// dir returns the vigie config directory, honoring XDG_CONFIG_HOME on Linux.
func dir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(d, "vigie"), nil
}

// legacyDirPath returns a path under the pre-rename ~/.config/claude-fleet
// directory, so a config written before the vigie rename is still read.
func legacyDirPath(name string) (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(d, "claude-fleet", name), nil
}

// envConfigPath returns the config-path override from the environment: VIGIE_CONFIG
// when set, else the deprecated FLEET_CONFIG (still honored for hooks installed
// before the rename). Empty when neither is set — the production leg (#289).
func envConfigPath() string {
	if p := os.Getenv("VIGIE_CONFIG"); p != "" {
		return p
	}
	return os.Getenv("FLEET_CONFIG")
}

// Path returns the config file path. VIGIE_CONFIG (or the deprecated
// FLEET_CONFIG), when set, overrides it with an explicit file (used by dev runs
// to target a local server without touching the installed production config).
func Path() (string, error) {
	if p := envConfigPath(); p != "" {
		return p, nil
	}
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// legacyPath returns the pre-TOML JSON config path.
func legacyPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads and parses the config file. When config.toml is absent, a pre-TOML
// config.json is migrated to config.toml on first load (see migrateLegacy).
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && envConfigPath() == "" {
		// Read-fallback: a config.toml written under the pre-rename
		// ~/.config/claude-fleet directory is still honored (soft migration).
		if old, oErr := legacyDirPath("config.toml"); oErr == nil {
			if d2, e2 := os.ReadFile(old); e2 == nil {
				data, err = d2, nil
			}
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		// Legacy config.json migration applies only to the default path, not to
		// an explicit VIGIE_CONFIG / FLEET_CONFIG override.
		if envConfigPath() == "" {
			if cfg, ok, mErr := migrateLegacy(); mErr != nil {
				return nil, mErr
			} else if ok {
				return cfg, nil
			}
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes the config as TOML with 0600 permissions (it holds a secret),
// creating the parent directory if needed. It returns the written path.
//
// The chmod after the write is not redundant: os.WriteFile applies its mode when
// it *creates* the file, so a config.toml left at 0644 by an older build keeps
// those bits and the token stays readable by every local account on the machine.
// The daemon tightens its existing database for the same reason (#526,
// store.restrictToOwner) — the operators who need it are the ones already running
// a file an earlier version created (#560).
func Save(cfg *Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing config %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("restricting config %s: %w", path, err)
	}
	return path, nil
}

// migrateLegacy converts a pre-TOML config.json to config.toml, once, and
// removes the JSON file so only the TOML config remains. It returns ok=false
// when no legacy file exists.
func migrateLegacy() (*Config, bool, error) {
	lpath, err := legacyPath()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(lpath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading legacy config %s: %w", lpath, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false, fmt.Errorf("parsing legacy config %s: %w", lpath, err)
	}
	if _, err := Save(&cfg); err != nil {
		return nil, false, err
	}
	if err := os.Remove(lpath); err != nil {
		return nil, false, fmt.Errorf("removing legacy config %s: %w", lpath, err)
	}
	return &cfg, true, nil
}

// OSUser returns the $USER environment value (the OS account name), or "".
func OSUser() string { return os.Getenv("USER") }

// EnvConfigPath returns the config-path override ("" when unset), which selects
// the dev vs production config leg: VIGIE_CONFIG, or the deprecated FLEET_CONFIG.
func EnvConfigPath() string { return envConfigPath() }
