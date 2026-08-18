package docs_test

import (
	"os"
	"path/filepath"
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

// #585. The guard above compares the server's mux to the map that documents it.
// Nothing compared it to the *client*, which spells every route as its own string
// literal in another package — `internal/report` posts to "/api/report" while
// `internal/server` registers "POST /api/report", and the two agree only because
// nobody has renamed one.
//
// Rename the server's route and update architecture.md, and the guard above
// passes, `internal/server` passes, and `internal/report` passes too: its tests
// talk to an httptest handler the test itself wrote, so they assert the path the
// client used matches what the *test author* expected. The build is green and no
// hook report ever lands again — `waiting`, the one status only a hook can see,
// goes permanently invisible with nothing saying why. The same joint-not-the-parts
// failure as #510 and #513.
//
// WHAT THIS DOES NOT COVER: the verb — the client's method is implicit in the
// helper it calls (`getJSON`, `postJSON`, `Get[T]`), so matching GET against POST
// would need call graph analysis. And only client → server: a route the server
// registers and no Go client calls is legitimate, because the web dashboard is
// JavaScript.

// clientRoutes are the /api paths the client packages name, each mapped to the
// files naming it — a failure has to say where to look, not just what is wrong.
//
// The package list is derived from `go list -deps ./cmd/vigie` (clientPackages,
// #556) rather than hand-kept, for the reason that list exists: a hand-kept list
// of what to check is not a check.
func clientRoutes(t *testing.T) map[string][]string {
	t.Helper()
	literal := regexp.MustCompile(`"(/api/[a-z/]+)"`)
	out := map[string][]string{}
	for _, pkg := range clientPackages(t) {
		dir := filepath.Join("../..", "internal", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v — the extraction is broken, not the code", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, m := range literal.FindAllStringSubmatch(read(t, filepath.Join(dir, name)), -1) {
				out[m[1]] = append(out[m[1]], pkg+"/"+name)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no /api literal found in any client package — the extraction is broken, not the code")
	}
	return out
}

// dynamicAPIPath matches an /api path being *built* rather than written whole.
// The extraction above reads literals, so a path assembled from pieces is
// invisible to it — and a guard that silently covers less than it claims is what
// #561 was about.
//
// Asserting instead that some known path is still found does not work, and was
// tried: `/api/report` is named by two packages, so one of them can stop naming
// it with the check none the wiser. The assumption to guard is not "this path is
// still visible" but "no path is built", which was verified true when this was
// written and is what keeps the extraction complete.
var dynamicAPIPath = regexp.MustCompile(`"/api[a-z/]*"\s*\+|Sprintf\(\s*"/api`)

func TestNoClientPackageBuildsAnAPIPath(t *testing.T) {
	for _, pkg := range clientPackages(t) {
		dir := filepath.Join("../..", "internal", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			if m := dynamicAPIPath.FindString(read(t, filepath.Join(dir, name))); m != "" {
				t.Errorf("%s/%s builds an API path (%q) instead of writing it whole, "+
					"which hides it from TestEveryRouteTheClientCallsIsOneTheServerRegisters", pkg, name, m)
			}
		}
	}
}

func TestEveryRouteTheClientCallsIsOneTheServerRegisters(t *testing.T) {
	registered := map[string]bool{}
	for route := range muxRoutes(t) {
		_, path, _ := strings.Cut(route, " ") // "POST /api/report" → "/api/report"
		registered[path] = true
	}
	for path, where := range clientRoutes(t) {
		if !registered[path] {
			t.Errorf("the client calls %s (%s) and the server registers no such route",
				path, strings.Join(where, ", "))
		}
	}
}
