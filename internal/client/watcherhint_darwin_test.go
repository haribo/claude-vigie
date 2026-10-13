package client

import (
	"os"
	"strings"
	"testing"
)

// The restart hint names a LaunchAgent label, and that label is defined only by
// the plist in docs/deployment.md (#869). A hint naming another label restarts nothing.
func TestWatcherRestartNamesTheDocumentedLaunchAgent(t *testing.T) {
	doc, err := os.ReadFile("../../docs/deployment.md")
	if err != nil {
		t.Fatal(err)
	}
	label := watcherRestart[strings.LastIndex(watcherRestart, "/")+1:]
	if !strings.Contains(string(doc), "<string>"+label+"</string>") {
		t.Errorf("docs/deployment.md defines no LaunchAgent labeled %q, which `vigie init` tells the operator to restart", label)
	}
}
