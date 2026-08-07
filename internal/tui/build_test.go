package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// TestRenderBuild covers the #341 Settings "Build" section: the client version
// always shows; the daemon reads "unknown" until fetched; a differing daemon
// version raises the drift warning, a matching one does not.
func TestRenderBuild(t *testing.T) {
	// Daemon not reached yet.
	out := model{}.renderBuild()
	if !strings.Contains(out, "vigie (this client)") {
		t.Errorf("missing client version row:\n%s", out)
	}
	if !strings.Contains(out, "unknown — not reached yet") {
		t.Errorf("daemon should read unknown before it is fetched:\n%s", out)
	}

	// Daemon differs → drift warning.
	out = model{daemonVersion: api.VersionInfo{Version: "9.9.9"}}.renderBuild()
	if !strings.Contains(out, "client and daemon versions differ") {
		t.Errorf("expected the drift warning:\n%s", out)
	}

	// Daemon matches the client (version.Version is "dev" under test) → no warning.
	out = model{daemonVersion: api.VersionInfo{Version: version.Version}}.renderBuild()
	if strings.Contains(out, "differ") {
		t.Errorf("no drift expected when versions match:\n%s", out)
	}
}
