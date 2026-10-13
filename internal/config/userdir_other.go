//go:build !darwin

package config

import "os"

// UserDir is the base directory for per-user config: os.UserConfigDir, which on
// Linux already honors XDG_CONFIG_HOME.
func UserDir() (string, error) { return os.UserConfigDir() }
