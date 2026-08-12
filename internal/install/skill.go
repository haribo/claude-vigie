package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// The session-raised call (ADR-0010) only fires if Claude knows the command
// exists. vigie therefore installs a personal Agent Skill, which is active in
// every project with no per-project setup — see
// docs/design/call-discoverability.md (#391).

// skillName is the directory vigie owns under the personal skills directory.
const skillName = "vigie-call"

// SkillPath returns the personal skill's SKILL.md path.
func SkillPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills", skillName, "SKILL.md"), nil
}

// skillDoc is the skill vigie writes. The `description` is the matching signal
// Claude uses to decide when to load it, so it is phrased the way an operator
// actually asks; the body says plainly that the call is best-effort.
const skillDoc = `---
name: vigie-call
description: Tell the operator through vigie that work is finished. Use when the user asks to be told, pinged, notified, or called when a long task completes — for example "tell me in vigie when you're done", "ping me in vigie once the migration finishes", or "let me know in vigie".
---

<!-- Managed by vigie. This file is rewritten on install and on watcher startup:
     local edits will be lost. See docs/design/call-discoverability.md. -->

# Call the operator in vigie

vigie is a dashboard the operator watches across many Claude Code sessions. When
they have asked to be told that something is finished, raise a call at the end of
the turn:

` + "```bash" + `
vigie call "backfill done — 12k rows, 0 errors"
` + "```" + `

## When to run it

Run it **once, at the very end of the turn**, when the work the operator asked
about is actually complete — not at the start, and not between steps.

Only run it when the operator asked to be told. It is a signal for them, not a
progress log.

## The message

Optional, but useful: it is displayed next to the session in the dashboard. One
short line saying what finished and how it went. A call with no message is still
a call.

## Notes

- The command exits 0 whatever happens — an unreachable server or a missing
  config can never fail the session. It prints nothing on success.
- The call clears by itself when the operator sends their next message in this
  session, so there is nothing to undo.
- This is best-effort: if it is not run, nothing is raised and the session simply
  looks as it always did.
`

// InstallSkill writes (or refreshes) the personal skill and returns its path.
// The directory is vigie's own, so the file is replaced wholesale.
func InstallSkill() (string, error) {
	path, err := SkillPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("creating skill dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(skillDoc), 0o600); err != nil {
		return "", fmt.Errorf("writing skill: %w", err)
	}
	return path, nil
}

// UninstallSkill removes the skill directory. An absent skill is not an error.
func UninstallSkill() (string, error) {
	path, err := SkillPath()
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("removing skill: %w", err)
	}
	return path, nil
}
