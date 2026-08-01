// Package web serves the read-only web dashboard: static assets embedded in the
// daemon binary (go:embed), a browser mirror of the terminal UI. The app shell
// is unauthenticated and holds no fleet data; the JavaScript authenticates its
// API calls with a token the operator pastes (kept in the browser). Same-origin
// only, strict CSP. Only the daemon imports this package.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var assets embed.FS

// contentSecurityPolicy locks the app to its own origin: scripts, styles, and
// connections may only come from the daemon that served the page, and the page
// may not be framed. Scripts and styles are external files, so no 'unsafe-inline'
// is needed — an injected <script> cannot run, which matters because the app
// holds a full-access token (see the package doc / issue #161).
const contentSecurityPolicy = "default-src 'self'; base-uri 'none'; " +
	"frame-ancestors 'none'; object-src 'none'; img-src 'self' data:"

// Handler serves the dashboard: the app shell at "/" and its assets under
// "/static/". Every response carries the CSP and hardening headers. Register it
// on the API mux for the unauthenticated GET "/" and "/static/" routes — the
// shell must load before the operator has pasted a token.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err) // the embedded static/ tree is compiled in; this cannot fail
	}
	// Read the shell once. Serving it via http.FileServer would 301-redirect
	// "/index.html" to "./", so write the bytes directly for the exact root.
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}
	static := http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secureHeaders(w)
		if r.URL.Path == "/" {
			// The shell must always render (hash-routed SPA); never let a stale
			// cache pin an old bundle after a daemon upgrade.
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// secureHeaders applies the CSP and defense-in-depth headers to a web response.
func secureHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
}
