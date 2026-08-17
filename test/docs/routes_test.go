package docs_test

import (
	"regexp"
	"strings"
	"testing"
)

// #511. `architecture.md` calls itself the top-level map, and its API table
// omitted `GET /api/version` and `POST /api/watcher/heartbeat` — the two routes
// that decide whether a watcher may write. The design docs had them; the map did
// not.
//
// Same shape as the command-table guard (#510) and the animation guard (#505):
// the checks this repository already had verify how a document is *structured*,
// never whether it describes the thing it claims to. That is what lets a table go
// two releases out of date with a green build.
//
// WHAT THIS DOES NOT COVER: the purposes. Prose cannot be compared without
// writing it twice. What is compared is the *set* of routes — the part a reader
// acts on, and the part that silently gains or loses an entry.

// muxRoutes are the routes the server registers, read from the mux itself.
func muxRoutes(t *testing.T) map[string]bool {
	t.Helper()
	body := read(t, "../../internal/server/server.go")
	pat := regexp.MustCompile(`mux\.Handle\("((?:GET|POST) /api/[a-z/]+)"`)
	out := map[string]bool{}
	for _, m := range pat.FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("no /api route found in server.go — the extraction is broken, not the doc")
	}
	return out
}

// documentedRoutes are the rows of architecture.md's "Server API" table.
func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	body := read(t, "../../docs/architecture.md")
	i := strings.Index(body, "## Server API")
	if i < 0 {
		t.Fatal("architecture.md has no `## Server API` section — this guard needs updating")
	}
	// | GET · POST | `/api/usage` | … — a row may document both verbs at once.
	row := regexp.MustCompile(`(?m)^\|\s*([A-Z· ]+)\s*\|\s*` + "`" + `(/api/[a-z/]+)` + "`")
	out := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(body[i:], -1) {
		for _, verb := range strings.Split(m[1], "·") {
			verb = strings.TrimSpace(verb)
			if verb == "GET" || verb == "POST" {
				out[verb+" "+m[2]] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no route row parsed out of the Server API table")
	}
	return out
}

func TestTheApiTableMatchesTheRoutesTheServerRegisters(t *testing.T) {
	mux, doc := muxRoutes(t), documentedRoutes(t)

	for route := range mux {
		if !doc[route] {
			t.Errorf("the server serves %q and architecture.md's API table does not list it", route)
		}
	}
	for route := range doc {
		if !mux[route] {
			t.Errorf("architecture.md's API table lists %q, which the server does not serve", route)
		}
	}
}
