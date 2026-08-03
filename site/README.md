# claudevigie.org — landing site

The public landing page for claude-vigie. A single static `index.html` (inline
CSS/JS, a `<canvas>` radar, no framework, no build step) plus `assets/`.

This is **only the marketing site content**. It is unrelated to `vigied`, the
daemon that serves the in-product web dashboard (`internal/web/`) — do not
confuse the two.

## Preview locally

```bash
cd site && python3 -m http.server 8000   # then open http://localhost:8000
```

Hosting and deployment of the site live outside this repository.
