package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestIndexIncludesPWAInstallHooks(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	app := servedBody(t, handler, "/app.js")

	for _, want := range []string{
		`<meta name="theme-color" content="#0e1116" />`,
		`<link rel="manifest" href="/manifest.json" />`,
		`<link rel="apple-touch-icon" href="/icons/icon-180.png" />`,
		`<link rel="stylesheet" href="/app.css" />`,
		`<script type="module" src="/app.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	if !strings.Contains(app, `navigator.serviceWorker.register("/sw.js")`) {
		t.Fatal("app script missing service worker registration")
	}
}

func TestIndexIncludesVoiceTranscribeControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	app := servedBody(t, handler, "/app.js")
	voice := servedBody(t, handler, "/js/voice.js")
	api := servedBody(t, handler, "/js/api.js")
	scripts := app + "\n" + voice

	for _, want := range []string{
		`id="record-btn"`,
		`id="rec-send"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	for _, want := range []string{
		`MediaRecorder`,
		`navigator.mediaDevices.getUserMedia`,
		`insertTranscript`,
		`finishRecordingConfirm`,
		`startRecording({ confirmOnStop: true })`,
		`startRecording({ confirmOnStop: false })`,
		`suppressRecordTextInput`,
	} {
		if !strings.Contains(scripts, want) {
			t.Fatalf("voice scripts missing %q", want)
		}
	}
	if !strings.Contains(api, `"/api/transcribe"`) {
		t.Fatal("api module missing transcribe endpoint")
	}
}

func TestIndexIncludesMobileUploadControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	style := servedBody(t, handler, "/app.css")
	app := servedBody(t, handler, "/app.js")
	help := servedBody(t, handler, "/js/help.js")
	api := servedBody(t, handler, "/js/api.js")
	scripts := app + "\n" + help

	for _, want := range []string{
		`id="upload-btn"`,
		`id="selection-btn"`,
		`id="clear-pane-btn"`,
		`id="file-upload"`,
		`id="file-upload" type="file" multiple hidden`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	for _, want := range []string{
		`openFileUploadPicker`,
		`upload-btn").addEventListener("click"`,
		`selection-btn").addEventListener("click", toggleSelectionMode)`,
		`clear-pane-btn").addEventListener("click", clearPaneOutput)`,
		`wsSend({ t: "clear" })`,
		`e.metaKey && !e.ctrlKey && !e.altKey && k === "k"`,
		`e.ctrlKey && !e.metaKey && !e.altKey && k === "l"`,
		`const files = Array.from(e.target.files || [])`,
		`e.key === "Tab" && e.shiftKey`,
		`return { t: "key", k: "BTab" }`,
		`if (state.selectionMode)`,
		`!draft.value.trim()`,
		`sendDirect({ t: "key", k: "Enter" })`,
		`selection mode so direct mode does not need focus/selection heuristics`,
		`!state.selectionMode && document.activeElement === $("direct-input")`,
		`tone: "upload"`,
		`tone: "selection"`,
		`tone: "clear"`,
		`tone: "settings"`,
	} {
		if !strings.Contains(scripts, want) {
			t.Fatalf("app/help scripts missing %q", want)
		}
	}
	if !strings.Contains(api, `"/api/upload-file"`) {
		t.Fatal("api module missing upload endpoint")
	}
	for _, want := range []string{
		`@media (any-pointer: coarse)`,
		`.key-area { display: flex; }`,
		`.selection-btn`,
		`.selection-btn.active`,
		`.clear-pane-btn`,
		`.content-wrap.selection-mode::after`,
		`.content-wrap.selection-mode pre#content`,
		`user-select: none`,
		`user-select: text`,
		`.help-ring.tone-upload`,
		`.help-ring.tone-clear`,
		`.help-tip.tone-settings`,
		`bottom: 60px;`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("style sheet missing %q", want)
		}
	}
}

func TestIndexIncludesSettingsControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	style := servedBody(t, handler, "/app.css")
	api := servedBody(t, handler, "/js/api.js")

	for _, want := range []string{
		`id="gear-btn"`,
		`id="settings-overlay"`,
		`id="font-range"`,
		`id="running-effect"`,
		`id="running-effect-preview"`,
		`id="build-time"`,
		`Build Time`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	if !strings.Contains(style, `--pane-font`) {
		t.Fatal("app stylesheet missing pane font variable")
	}
	if !strings.Contains(style, `.effect-preview`) {
		t.Fatal("app stylesheet missing running effect preview")
	}
	if !strings.Contains(api, `"/api/settings/stt"`) {
		t.Fatal("api module missing settings API call")
	}
	if !strings.Contains(api, `"/api/version"`) {
		t.Fatal("api module missing version API call")
	}
	settings := servedBody(t, handler, "/js/settings.js")
	if !strings.Contains(settings, `applyRunningEffect`) {
		t.Fatal("settings script missing running effect setting")
	}
	if !strings.Contains(settings, `loadVersionInfo`) {
		t.Fatal("settings script missing version info loader")
	}
}

func TestAppIncludesAgentChipIconsAndAsciiRules(t *testing.T) {
	handler := (&Server{}).Handler()
	app := servedBody(t, handler, "/app.js")
	terminal := servedBody(t, handler, "/js/terminal.js")
	style := servedBody(t, handler, "/app.css")

	for _, want := range []string{
		`const RUNTIME_ICON = { claude: "cc", codex: "cx", copilot: "cp", gemini: "g" }`,
		`function paneIndicator(p)`,
		`class: cls.join(" ")`,
		`if (!p.asking) return null;`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing %q", want)
		}
	}
	for _, want := range []string{
		`/^[─-]+$/`,
		`lines[i] = RULE_OPEN + RULE_CLOSE;`,
		`role="separator"`,
		`IMAGE_PATH_RE`,
		`span.className = "image-path"`,
		`span.dataset.path`,
		`const URL_CHARS =`,
		`new RegExp(`,
		`function extractURLs(text)`,
		`.replace(URL_ANSI_RE, "")`,
		`.replace(/\n[ \t]+/g, "")`,
		`class="tui-link"`,
		`export function measurePaneSize()`,
	} {
		if !strings.Contains(terminal, want) {
			t.Fatalf("terminal module missing %q", want)
		}
	}
	for _, want := range []string{
		`measurePaneSize`,
		`{ t: "resize", cols: sz.cols, rows: sz.rows }`,
		`window.addEventListener("resize", scheduleResize)`,
		`window.visualViewport.addEventListener("resize", scheduleResize)`,
		`lastSentSize = { cols: 0, rows: 0 }`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing resize wiring %q", want)
		}
	}
	for _, want := range []string{
		`previewImagePath`,
		`new URLSearchParams({ path })`,
		`e.target.closest(".image-path")`,
		`!e.metaKey`,
		`IMAGE_LONG_PRESS_MS`,
		`pointerdown`,
		`pointercancel`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing %q", want)
		}
	}
	for _, want := range []string{
		`--tmact-vvh`,
		`scheduleFitViewport`,
		`document.addEventListener("focusin", scheduleFitViewport)`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing viewport keyboard guard %q", want)
		}
	}
	if strings.Contains(app, `pre.scrollTop = pre.scrollHeight`) {
		t.Fatal("app script must not scroll pane output to the blank tmux tail on keyboard resize")
	}
	for _, want := range []string{
		`.agent-icon.runtime-claude`,
		`.agent-icon.runtime-codex`,
		`.agent-icon.runtime-copilot`,
		`.agent-icon.runtime-gemini`,
		`.image-path`,
		`.image-preview`,
		`.image-preview img`,
		`height: var(--tmact-vvh, 100dvh);`,
		`overflow: hidden;`,
		`@keyframes agent-shine`,
		`@keyframes agent-rainbow`,
		`display: block;`,
		`border-top: 1px solid var(--border);`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("app style missing %q", want)
		}
	}
}

func TestPWAAssetsContentTypes(t *testing.T) {
	handler := (&Server{}).Handler()
	tests := []struct {
		path        string
		contentType string
	}{
		{"/manifest.json", "application/json"},
		{"/app.css", "text/css"},
		{"/app.js", "text/javascript"},
		{"/sw.js", "text/javascript"},
		{"/icons/icon-180.png", "image/png"},
		{"/icons/icon-192.png", "image/png"},
		{"/icons/icon-512.png", "image/png"},
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

func servedBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200", path, rec.Code)
	}
	return rec.Body.String()
}

func TestServiceWorkerBypassesLiveEndpoints(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`url.pathname.startsWith("/api/")`,
		`url.pathname.startsWith("/ws/")`,
		`APP_SHELL_PATHS.has(url.pathname)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("service worker missing %q", want)
		}
	}
}
