package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The browser UI is a Vite + React + TypeScript app under internal/web/frontend/
// whose production build is emitted into internal/web/static/ (embedded by
// go:embed). These tests assert what the SERVER genuinely guarantees about the
// BUILT app — the index shell, the hashed /assets/* bundle, the PWA service
// worker, manifest, and icons. Behavioral parity of the JS itself is covered by
// the frontend Vitest suite (src/**/*.test.ts), not by grepping served source.
//
// On a fresh checkout where `make web` has not run, internal/web/static holds
// only .gitkeep; requireBuilt() skips the build-dependent tests so `go test`
// still passes without the Node toolchain. `make test` builds first, so CI runs
// them for real.

func requireBuilt(t *testing.T) {
	t.Helper()
	b, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil || !strings.Contains(string(b), `id="root"`) {
		t.Skip("React UI not built into internal/web/static — run `make web` first")
	}
}

func servedBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200", path, rec.Code)
	}
	return rec.Body.String()
}

var (
	moduleScriptRe = regexp.MustCompile(`<script[^>]+type="module"[^>]+src="(/assets/[^"]+\.js)"`)
	stylesheetRe   = regexp.MustCompile(`<link[^>]+rel="stylesheet"[^>]+href="(/assets/[^"]+\.css)"`)
)

// builtAssetPaths parses the served index.html for the hashed JS + CSS bundle
// paths Vite injected. The names contain a content hash, so callers must
// discover them rather than hard-code them.
func builtAssetPaths(t *testing.T, handler http.Handler) (jsPath, cssPath string) {
	t.Helper()
	index := servedBody(t, handler, "/")
	js := moduleScriptRe.FindStringSubmatch(index)
	if js == nil {
		t.Fatalf("index.html has no <script type=module src=/assets/*.js>:\n%s", index)
	}
	css := stylesheetRe.FindStringSubmatch(index)
	if css == nil {
		t.Fatalf("index.html has no <link rel=stylesheet href=/assets/*.css>:\n%s", index)
	}
	return js[1], css[1]
}

func TestIndexPageServed(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>tmact") {
		t.Fatal("index page body missing expected title")
	}
}

func TestIndexServesReactAppShell(t *testing.T) {
	requireBuilt(t)
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")

	for _, want := range []string{
		`<div id="root">`,
		`<meta name="theme-color" content="#0e1116" />`,
		`<link rel="manifest" href="/manifest.json" />`,
		`<link rel="apple-touch-icon" href="/icons/icon-180.png" />`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index shell missing %q", want)
		}
	}
	if !moduleScriptRe.MatchString(body) {
		t.Fatal("index shell missing hashed module <script src=/assets/*.js>")
	}
	if !stylesheetRe.MatchString(body) {
		t.Fatal("index shell missing hashed <link rel=stylesheet href=/assets/*.css>")
	}
}

// The hand-written shell's DOM ids and API endpoints used to be asserted against
// the served source. React renders the DOM at runtime, so we assert their string
// literals survive in the bundled JS — a cheap smoke that every feature shipped.
func TestBundledAppContainsControlsAndEndpoints(t *testing.T) {
	requireBuilt(t)
	handler := (&Server{}).Handler()
	jsPath, _ := builtAssetPaths(t, handler)
	js := servedBody(t, handler, jsPath)

	// PWA service worker registration.
	if !strings.Contains(js, "/sw.js") {
		t.Fatal("bundled app missing service worker registration (/sw.js)")
	}
	// Every server endpoint the UI speaks to (string literals survive minify).
	for _, want := range []string{
		"/api/snapshot", "/api/snapshot/stream", "/api/version",
		"/api/agent-usage", "/api/settings/stt", "/api/transcribe",
		"/api/paste-image", "/api/upload-file", "/api/image", "/ws/pane",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("bundled app missing endpoint %q", want)
		}
	}
	// Control element ids the original shell shipped (JSX id="..." → string lit).
	for _, want := range []string{
		"record-btn", "rec-send", "upload-btn", "selection-btn", "clear-pane-btn",
		"file-upload", "gear-btn", "settings-overlay", "running-effect", "build-time",
		"qb-fab", "help-btn", "stale-dot", "conn-status", "option-bar", "direct-input",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("bundled app missing control id %q", want)
		}
	}
	// statusd owns the tmux window size; the browser must NOT measure its viewport
	// and report it over the WS (it re-introduces rag-edged shared scrollback).
	if strings.Contains(js, "measurePaneSize") {
		t.Fatal("bundled app must not include pane-size measurement / resize wiring")
	}
}

func TestBundledStyleHasRuntimeClasses(t *testing.T) {
	requireBuilt(t)
	handler := (&Server{}).Handler()
	_, cssPath := builtAssetPaths(t, handler)
	css := servedBody(t, handler, cssPath)

	// Class names, keyframe ids and custom properties are not renamed by CSS
	// minification, so these survive the build verbatim.
	for _, want := range []string{
		".agent-icon", "runtime-claude", "runtime-codex", "runtime-copilot",
		"runtime-gemini", "agent-shine", "agent-rainbow", "--pane-font",
		"--tmact-vvh", ".image-preview", ".image-path", ".selection-btn",
		".clear-pane-btn", ".effect-preview",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("bundled stylesheet missing %q", want)
		}
	}
}

func TestServedAssetsContentTypes(t *testing.T) {
	requireBuilt(t)
	handler := (&Server{}).Handler()
	jsPath, cssPath := builtAssetPaths(t, handler)

	tests := []struct {
		path        string
		contentType string
	}{
		{"/manifest.json", "application/json"},
		{"/sw.js", "text/javascript"},
		{"/icons/icon-180.png", "image/png"},
		{"/icons/icon-192.png", "image/png"},
		{"/icons/icon-512.png", "image/png"},
		{jsPath, "text/javascript"},
		{cssPath, "text/css"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", got, tt.contentType)
			}
		})
	}
}

// The Go server rewrites sw.js's CACHE_NAME suffix to a content hash of every
// embedded static file, so any rebuilt asset busts the offline cache without a
// manual bump. Verify the rewrite fired and matches /api/version's asset_hash.
func TestServiceWorkerCacheNameMatchesAssetHash(t *testing.T) {
	requireBuilt(t)
	handler := (&Server{}).Handler()
	sw := servedBody(t, handler, "/sw.js")

	if strings.Contains(sw, "tmact-app-shell-vDEV") {
		t.Fatal("sw.js CACHE_NAME still the literal vDEV — server rewrite did not fire")
	}
	m := regexp.MustCompile(`tmact-app-shell-([0-9a-f]{12})`).FindStringSubmatch(sw)
	if m == nil {
		t.Fatal("sw.js CACHE_NAME not rewritten to a 12-hex content hash")
	}
	if ver := servedBody(t, handler, "/api/version"); !strings.Contains(ver, m[1]) {
		t.Fatalf("/api/version asset_hash does not match sw cache hash %q: %s", m[1], ver)
	}
}

func TestServiceWorkerBypassesLiveEndpoints(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/sw.js status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`url.pathname.startsWith("/api/")`,
		`url.pathname.startsWith("/ws/")`,
		`isShellPath(url.pathname)`,
		`startsWith("/assets/")`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("service worker missing %q", want)
		}
	}
}
