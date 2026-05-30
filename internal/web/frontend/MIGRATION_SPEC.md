# tmact Web Frontend Migration Spec: Vanilla JS (ES modules) → React + TypeScript

**Goal:** Re-implement the tmact statusd web UI in React + TypeScript with EXACT behavioral parity. The Go server is the immutable contract boundary — it is NOT changing. Built output must continue to be embedded via `go:embed static` and consumed by `http.FileServer`, with the existing service-worker / asset-hash / manifest mechanism preserved.

---

## 1. Overview

`tmact statusd`'s web UI is a single-page, PWA-capable, mobile-first console for driving terminal AI-agent panes (Codex / Claude / Copilot / Gemini) over tmux. It lists tmux panes as chips, streams a selected pane's live output over a WebSocket, classifies runtime + idle/asking/running state, and relays keyboard/text/voice/file input back into the pane. The web UI's input path is an intentional **live-send surface** (unlike the dry-run-by-default CLI), gated by a server-side key allowlist and validated pane ids.

**Full feature list (one bullet per feature):**

- Snapshot polling (1000 ms) and SSE subscription to `/api/snapshot/stream`, with ETag-cached `/api/snapshot` HTTP fallback (304 → cached snapshot).
- Stale-connection detection: red `#stale-dot` when snapshot is older than `STALE_MS = 10000` ms.
- Status-line chips: one per pane, sorted by peer → session label → window_index → pane_index, with peer badge, runtime icon (cc/cx/cp/g), state class (stale/asking/running/idle), Option-key hotkey label, and `.sel` for the selected pane.
- Pane selection + restoration: persists `{pane, session}` to `localStorage["tmact.selectedPane"]`; restores once on first snapshot (exact pane_id match, falls back to session-name match).
- Per-pane WebSocket stream (`/ws/pane?pane=<id>`): patch application (`paneLines[from:]` splice), error display, interactive-question rendering, exponential backoff (1 s→30 s, reset after 5 s stable or on pane switch), and `connecting/reconnecting/open` connection-status strip.
- Pane output rendering (`setContent`): ANSI SGR → HTML, URL detection/wrapping (incl. soft-wrap joining), box-art table extraction, horizontal-rule replacement, image-path marking, private-use-Unicode placeholders, and auto-scroll-if-within-60px-of-bottom.
- Stale-while-revalidate `paneCache` keyed by pane id (no "Connecting…" flash on re-select).
- Draft textarea input mode: per-pane draft cache (`state.drafts`), auto-grow (scrollHeight + border, cap 200 px), Ctrl/Cmd+Enter send, Shift+Enter newline, empty-Enter → direct-mode, image paste → insert path, clear button.
- Direct keystroke-passthrough mode (invisible `#direct-input` overlay): `translateKey` mapping, sticky Ctrl arming, IME/composition handling, soft-keyboard `input` relay, meta-key blocking, paste handling.
- Keyboard hotkeys (desktop only): Option+`1…0`/`q…p` (layout-independent via `KeyboardEvent.code`) to select chips; Cmd+K / Ctrl+L to clear pane output.
- On-screen helper key bar (mobile): Esc, ^C, Tab, ⇧Tab, Ctrl-toggle (sticky), arrows, Home/End/PgUp/PgDn; single-row clipping with expand toggle.
- Quick-button FAB (phone): configurable per-runtime quick-send buttons, settings editor, `localStorage["tmact.quickButtons"]`.
- Selection mode: preserve text selection in `#content` for copy; copy-line bar (join-glue / join-space) with clipboard + `execCommand` fallback and 900 ms flash.
- Image long-press preview (mobile 550 ms / desktop Cmd+click) → `/api/image` lightbox.
- Option/quick-answer bar from detected pane questions.
- Voice recording + transcription: Option+V hotkey (confirm flow) and record button (auto-upload), MediaRecorder lifecycle, overlay states (recording/transcribing/confirming/hotkey-recording), 700 ms input suppression, draft insertion.
- File upload (picker) → `/api/upload-file`; clipboard image paste → `/api/paste-image`.
- Settings modal: font size (`--pane-font`), running-effect (`data-running-effect`) with live preview, STT server config (GET/PUT `/api/settings/stt`), quick-button editor, build/asset version info.
- Usage panel (top-right): agent quota/rate-limit per provider, 60 s polling of `/api/agent-usage`, local countdown recompute, 404 → stop + hide.
- Help coachmark overlay: rings + tip cards with collision-avoidance placement, visualViewport-aware.
- Mobile viewport fit: `--tmact-vvh` from `visualViewport.height`, iOS Safari timing quirks (rAF + 80 ms + 260 ms).
- Connection-status + mode-indicator + input-error strips.
- PWA: service worker (network-first app-shell cache, hash-versioned `CACHE_NAME`), manifest, icons.
- Window visibility lifecycle: stop polling/SSE/WS when hidden, resume when visible.

---

## 2. Server contract (frozen)

The Go server (`internal/web/*.go`) is immutable. The React app MUST speak exactly this contract. All `/api/*` responses set `Content-Type: application/json` and `Cache-Control: no-store`. Error responses are `{"error": "<msg>"}`.

### 2.1 Static + PWA shell

| Method | Path | Response |
|---|---|---|
| GET | `/` (and any non-API path) | `http.FileServer` over embedded `static/`. Serves `index.html`, `/app.css`, `/app.js`, `/js/*`, `/icons/*`, `/manifest.json`, etc., with Go default MIME detection. |
| GET/HEAD | `/sw.js` | `text/javascript; charset=utf-8`, `Cache-Control: no-store`. Server replaces every match of regex `tmact-app-shell-v[0-9A-Za-z]+` in the file body with `tmact-app-shell-<12-char-hash>`. Hash = first 12 hex chars of SHA256 over `path\x00 + contents + \0` for every file under `static/`. Computed once (`sync.Once`), cached for process lifetime. |
| GET | `/manifest.json` | PWA manifest: `name`/`short_name` `"tmact"`, `display: standalone`, `start_url: "/"`, `scope: "/"`, `theme_color`/`background_color` `#0e1116`, icons 180/192/512 (512 includes `maskable`). |

### 2.2 `GET /api/version`

```json
{ "build_time": "<ISO8601 or empty>", "asset_hash": "<12-char hex>" }
```
Method other than GET → 405 with `Allow: GET`.

### 2.3 `GET /api/snapshot`

503 `{"error":"snapshot store not configured"}` if no store; 503 `{"error":"snapshot not yet available"}` if no snapshot yet. Otherwise 200 JSON:

```ts
interface Snapshot {
  version: 1;
  ts: string;                 // ISO8601 (json tag in struct; spec uses "ts")
  generated_by: string;       // "tmact statusd"
  interval_ms: number;
  stale_after_ms: number;
  summary: {
    sessions: number; panes: number; working: number; asking: number; errors: number;
  };
  sessions: Record<string, SessionStatus>;
  panes: Record<string, PaneStatus>;   // keyed by target
  errors?: SnapshotError[];            // max 32
}
interface SessionStatus {
  session: string; id: string; active_target?: string; tag: string;
  runtime: string; state: string;
  running: boolean; asking: boolean; stale: boolean;
  row_bucket: number;          // 0..2 visual hint, NOT semantic
  updated_at: string; peer: string;    // peer empty for local
}
interface PaneStatus {
  target: string; pane_id: string; session: string;
  window_index: number; pane_index: number;
  cwd: string; current_command: string;
  runtime: string; tag: string; state: string;
  idle: boolean; input_ready: boolean; running: boolean; asking: boolean; stale: boolean;
  confidence: number; signals: string[];
  prompt: Question | null;     // interactive menu, null when none
  last_line: string;
  last_changed_at: string | null;   // *time.Time → null until first run, else RFC3339
  updated_at: string; error: string; peer: string;
}
interface Question { prompt?: string; choices: { number: number; label: string }[]; }
```
Note: `Cache-Control: no-store` — there is **no ETag** emitted by `handleSnapshot`. (The existing client's ETag/304 logic and `lastSnapshot` cache are a client-side optimization that must be preserved as a no-op-safe path: if the server never returns 304, the cache simply never serves stale; keep the `If-None-Match`/304 handling defensively but expect 200s.)

**State derivation rules (must replicate for UI classes):** `working` = `running && !asking`; `asking` = `pane.asking || prompt != null || state === "waiting_permission"`; pane `state` values: `working`/`idle`/`unknown`/`waiting_permission`. `summary.working` counts panes where `state === "working" || running`; `summary.asking` counts `asking`.

### 2.4 `GET /api/snapshot/stream` (SSE)

`Content-Type: text/event-stream`, `Connection: keep-alive`, `X-Accel-Buffering: no`. Primes with the current snapshot immediately on connect, then sends:

```
event: snapshot
data: {<Snapshot JSON>}

```
Between updates, raw keepalive comment lines `: ping\n\n` every 25 s (no event name, no JSON — `EventSource` ignores comments automatically). Client should not manually reconnect; rely on `EventSource` auto-reconnect, and fall back to polling on `onerror`.

### 2.5 `GET /api/agent-usage`

`{ cache: 'no-store' }`. 503 if not yet refreshed (first refresh fires on startup, then every 5 min). 404 if `UsageEnabled` is false → **client stops polling permanently and hides panel**.

```ts
interface AgentUsage {
  generated_at: string;
  providers: ProviderUsage[];
}
interface ProviderUsage {
  provider: "claude" | "codex" | string;
  account?: string; plan?: string;
  windows?: RateWindow[]; cost?: CostWindow | null; error?: string;
}
interface RateWindow {
  name: string;            // "session" | "weekly" | "weekly_opus" | ...
  used_percent: number;    // 0..100+, may exceed 100
  window_minutes?: number; resets_at?: string | null;
  pace?: Pace | null;
}
interface Pace {
  stage: "on_track"|"slightly_ahead"|"ahead"|"far_ahead"|"slightly_behind"|"behind"|"far_behind";
  delta_percent: number; expected_percent: number; actual_percent: number;
  eta_seconds?: number | null; lasts_until_reset: boolean;
}
interface CostWindow { enabled: boolean; used: number; limit: number; currency?: string; unlimited?: boolean; }
```

### 2.6 `GET /api/settings/stt`

```json
{ "model": "string", "endpoint": "string", "configured": boolean }
```
API key never returned. Defaults filled from `stt.DefaultModel`/`stt.DefaultEndpoint`; `configured = (APIKey != "")`.

### 2.7 `PUT /api/settings/stt`

Body (max 64 KB) `application/json`: `{ "model": string, "endpoint": string, "api_key": string }`. **Blank `api_key` = keep existing key (merge, not erase).** Model/endpoint trimmed. Returns the same shape as GET, after `NormalizeAndValidate()` (response may differ from input). Validation failure → 400 with reason; invalid JSON → 400.

### 2.8 `POST /api/transcribe`

`multipart/form-data`, field `audio` (binary). Max 25 MB (→ 413 if exceeded). Server sniffs container from magic bytes (WebM/Ogg/MP4/MP3/WAV/FLAC), ignores declared MIME. Calls external STT endpoint with `Authorization: Bearer <key>`, fields `model`, `response_format=json`, `file`. Returns `{ "text": "<trimmed transcript>" }`. 503 if not configured; empty transcript → 502; missing field → 400.

### 2.9 `POST /api/paste-image`

`multipart/form-data`, field `image`. Optional query `?peer=<encodeURIComponent(peer)>`. Max 25 MB. Sniffs PNG/JPEG/GIF/WebP/BMP/SVG; unsupported → 415. Saves `paste-<TS>-*.<ext>`. Returns `{ "path": "<absolute>" }`.

### 2.10 `POST /api/upload-file`

`multipart/form-data`, field `file` repeated for multiple files. Optional `?peer=`. Max 100 MB. Filename sanitized (alnum + `.-_`, collapse dashes, trim, cap 120 chars, preserve ext). Saves `upload-<TS>-*-<name>`. Returns `{ "path": "<first>", "paths": ["<...>"] }` (or just `paths` if none); rollback (delete) on partial failure → 500. Missing field → 400.

### 2.11 `GET /api/image`

Query `path` (required, trimmed), `cwd` (required+absolute if `path` is relative). Supports `file:///abs` and `file://localhost/abs`. Rejects `~/...`. Returns image binary with sniffed `Content-Type` (png/jpeg/gif/webp/bmp/svg+xml). Always `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`; SVG adds CSP `default-src 'none'; style-src 'unsafe-inline'`. `http.ServeContent` handles Range/ETag/Last-Modified. Bad scheme/relative-without-cwd/directory → 400; unreadable → 404; unsupported ext → 415.

### 2.12 `GET (WebSocket) /ws/pane?pane=<id|peer@id>`

`pane` required, must match `^(?:[A-Za-z0-9_.-]+@)?%[0-9]+$`. Invalid → HTTP 400 `{"error":"invalid 'pane' parameter, expected a tmux pane id like %12 or peer@%12"}`. `peer@`-prefixed ids look up `Peers`; unknown peer → 404 `{"error":"unknown peer '<name>'"}`; peer dial failure → 502; otherwise transparently proxies frames to the peer's `/ws/pane`.

Client → server (`inputMsg`):
```ts
type InputMsg =
  | { t: "text";  s: string }   // paste literal text, NO Enter (empty s ignored)
  | { t: "send";  s: string }   // paste text + Enter (empty s ignored)
  | { t: "key";   k: string }   // single allowlisted tmux key
  | { t: "clear" }              // clear pane + scrollback
  | { t: "resize" };            // legacy, silently ignored
```
Allowed `k`: `Enter, BSpace, Tab, BTab, Escape, Up, Down, Left, Right, Home, End, PageUp, PageDown, Delete, Space`, or `C-a`…`C-z` (lowercase only; `C-C`, `C-1`, `M-x` rejected). Disallowed → server sends an error outMsg (does NOT close).

Server → client (`outMsg`):
```ts
type OutMsg =
  | { t: "patch"; from: number; lines: string[]; q?: Question | null }
  | { t: "error"; s: string };
```
- Capture cadence 200 ms; buffer 400 lines (`wsCaptureLines`).
- **Initial patch on connect: `from = 0` with the full lines array** (even when empty → `from:0, lines:[]`). Client replaces entire buffer.
- Subsequent patches: longest-common-prefix line diff — `from = prefixCount`, `lines` = diverging tail only (string equality per line, NOT ANSI-aware). Empty `lines` with `from = previous length` = "no append".
- `q` rides on every patch: `Question` when an interactive menu is detected (cursor marker `❯` + numbered `1..N`), else omitted/null. **Preserve the EXISTING client behavior: clicking a choice sends `{ t: "text", s: String(choice.number) }`** (both `text` and `key` reach the pane; the original client uses `text`).
- Read limit 1 MiB/frame. App-level ping/pong every 25 s, 10 s timeout; browser `WebSocket` has no native ping API, so rely on the server closing dead connections and the client's own backoff.
- Errors do NOT close the socket; client must not treat an error as disconnect.

---

## 3. Proposed React + TS architecture

### 3.1 Build tool: Vite

Vite (React + TS) is chosen for fast HMR in dev and a static, hashable production bundle. **Critical constraint:** the Go server serves `static/` verbatim via `FileServer` and computes the SW cache hash by walking *every file under `static/`*, and rewrites only the `tmact-app-shell-v[0-9A-Za-z]+` literal in `sw.js`. The build must therefore:

- Emit a self-contained set of files into `internal/web/static/` (overwriting the current hand-written shell).
- Keep `/sw.js`, `/manifest.json`, and `/icons/*` at the SAME paths (root-level, not under `/assets/`), because the SW's `APP_SHELL_URLS` and the Go `/sw.js` route depend on them.
- Keep `sw.js` containing the literal `tmact-app-shell-vDEV` so the Go regex rewrite still fires (the SW is NOT bundled/renamed by Vite — it ships as a verbatim static file).

### 3.2 Directory layout

```
internal/web/
  frontend/                      # NEW — source, NOT embedded
    index.html                   # Vite entry; mirrors current <head> (manifest, icons, theme-color, viewport)
    package.json
    tsconfig.json
    vite.config.ts
    public/                      # copied verbatim to build root → static/
      sw.js                      # verbatim, contains "tmact-app-shell-vDEV"
      manifest.json
      icons/icon-180.png
      icons/icon-192.png
      icons/icon-512.png
    src/
      main.tsx                   # ReactDOM.createRoot + <App/> + SW registration
      app.css                    # ported 1:1 from current static/app.css (CSS vars, animations, media queries)
      types/server.ts            # server contract types (Snapshot, OutMsg, …)
      api/client.ts              # mirrors api.js
      ws/usePaneStream.ts        # mirrors stream.js
      ws/useSnapshotStream.ts    # mirrors api.js subscribeSnapshot + app.js polling/fallback
      store/AppStateContext.tsx  # mirrors state.js (state/voice/upload)
      terminal/render.ts         # PURE port of terminal.js (ANSI/URL/table/rule/image) — no React
      hooks/                     # useVoice useUpload useUsage useViewport useSettings useQuick useHotkeys
      lib/dom.ts                 # clamp, escapeHTML, isMobile
      lib/keymap.ts              # KEYMAP, HOTKEY_INDEX, translateKey
      components/                # App, StatusLine, Chip, ConnStatus, ContentPane, CopyLineBar,
                                 # ImagePreview, InputBar, Draft, DirectInput, KeyBar, QuickDock,
                                 # OptionBar, UsagePanel, SettingsDialog, QuickEditor, HelpOverlay, RecOverlay
  static/                        # BUILD OUTPUT — embedded via go:embed (regenerated by vite build)
  server.go                      # UNCHANGED
```

`internal/web/frontend/` is the source of truth; `make`/CI runs `vite build` whose `outDir` is `../static`. Because `go:embed static` requires the files to exist at `go build` time, a Makefile `generate` target runs the Vite build before `go build`.

### 3.3 `vite.config.ts` essentials

```ts
export default defineConfig({
  plugins: [react()],
  root: ".",
  build: {
    outDir: "../static",
    emptyOutDir: true,            // wipes old hand-written shell on each build
    assetsDir: "assets",          // hashed JS/CSS under static/assets/*
  },
});
```

- Vite copies `public/sw.js`, `public/manifest.json`, `public/icons/*` to `static/` root unchanged. The hashed JS/CSS land under `static/assets/`.
- **SW cache busting is preserved unchanged:** since the Go asset-hash walks ALL files under `static/`, any change to a hashed Vite asset flips the SHA256 hash, which the Go server injects into `sw.js`'s `CACHE_NAME`. No manual bump needed — identical to today.
- **SW app-shell list must be updated:** the hand-written `APP_SHELL_URLS` enumerating `/app.js` and `/js/*.js` no longer matches Vite's hashed asset names. Parity-safe approach: precache only the stable, known-path entries (`/`, `/index.html`, `/manifest.json`, `/icons/*`) and let the network-first fetch handler cache hashed `/assets/*` opportunistically (extend the path match to a `/assets/` prefix check instead of a `Set` membership test).
- The SW must keep the literal `tmact-app-shell-vDEV` token (so the Go regex matches) and keep skipping `/api/*` and `/ws/*` (unchanged).

### 3.4 `tsconfig.json` essentials

`"strict": true`, `"target": "ES2020"`, `"module": "ESNext"`, `"moduleResolution": "Bundler"`, `"jsx": "react-jsx"`, `"lib": ["ES2020","DOM","DOM.Iterable"]`, `"noUncheckedIndexedAccess": true`. DOM lib types for `visualViewport`, `MediaRecorder`, `EventSource`, `WebSocket`.

### 3.5 `index.html` (Vite entry)

Mirror the current `<head>`: `viewport` with `maximum-scale=1, viewport-fit=cover`, `theme-color #0e1116`, `apple-mobile-web-app-*`, `<link rel="manifest" href="/manifest.json">`, icon links, and a single `<div id="root">`. The `app.css` stays a global stylesheet (imported in `main.tsx`) so `--pane-font`, `data-running-effect`, `--tmact-vvh`, and all keyframes/media queries port unchanged — do NOT convert to CSS-in-JS.

### 3.6 SW registration

In `main.tsx`, on `window` `"load"`: `navigator.serviceWorker?.register("/sw.js").catch(() => {})` — fire-and-forget, errors silently ignored (parity).

---

## 4. Component tree

```
<App>                                  ← app.js (orchestration), main.tsx
 ├─ <AppStateProvider>                 ← state.js (state/voice/upload context)
 │   ├─ <ContentWrap id="content-wrap">           ← app.js content-wrap classes (.direct/.selection-mode/.upload-ready)
 │   │   ├─ <UsagePanel/>              ← usage.js
 │   │   ├─ <ContentPane/>             ← terminal.js (render) + app.js #content events
 │   │   │     ├─ <CopyLineBar/>       ← terminal.js copy bar + app.js wireCopyLine
 │   │   │     └─ <ImagePreview/>      ← app.js previewImagePath/openImageTarget (portal lightbox)
 │   │   ├─ <DirectInput/>             ← app.js direct-mode (translateKey/sendDirect/composition/soft-kbd)
 │   │   ├─ <HelpButton/> <UploadButton/> <SelectionButton/> <ClearPaneButton/>  ← app.js action buttons
 │   │   ├─ <QuickDock/>               ← quick.js (FAB + menu + backdrop)
 │   │   └─ (file input, hidden)       ← upload.js openFileUploadPicker
 │   ├─ <StatusBar id="statusline">
 │   │   ├─ <ConnStatus/>              ← app.js setConnStatus (#conn-status)
 │   │   ├─ <OptionBar/>               ← app.js renderOptions (#option-bar)
 │   │   ├─ <StatusLine id="chips">    ← app.js renderStatusline
 │   │   │     └─ <Chip/> ×N           ← one per pane (sort/hotkey/state/title)
 │   │   ├─ <StaleDot/>                ← app.js checkStale
 │   │   └─ <GearButton/>              ← settings.js open trigger
 │   ├─ <InputBar id="input-bar">      ← app.js wireInput
 │   │   ├─ <KeyBar/>                  ← app.js buildKeyBar/syncKeyBar
 │   │   ├─ <ModeIndicator/>           ← app.js renderMode/syncIndicator
 │   │   └─ <DraftRow/>                ← app.js draft mode (Draft, Send, Record, DraftClear)
 │   ├─ <RecOverlay/>                  ← voice.js
 │   ├─ <SettingsDialog/>              ← settings.js  (+ <QuickEditor/> ← quick.js)
 │   └─ <HelpOverlay/>                 ← help.js
 └─ (portals for image-preview, settings-overlay, help-overlay, rec-overlay)
```

**Absorption notes:**
- `dom.js` → `lib/dom.ts` (`clamp`, `escapeHTML`, `isMobile`); `escapeHTML` still required inside `terminal/render.ts`.
- `terminal.js` is ported as a **pure function** (`render(text, opts) → htmlString` + `markImagePaths` post-pass). `ContentPane` renders a childless `pre#content` and assigns its `innerHTML` imperatively in a `useLayoutEffect` (the pure renderer's output), then runs `markImagePaths`/scroll restoration. React must NOT diff the inner HTML.
- `api.js` → `api/client.ts`; `stream.js` → `ws/usePaneStream.ts`; voice/upload/usage/settings/quick/help each map to a hook + component pair.

---

## 5. Shared modules

### 5.1 Typed API client — `api/client.ts` (mirrors `api.js`)

Module-level `let snapshotEtag = ""`, `let lastSnapshot: Snapshot | null = null`. Functions return the SAME shapes/error strings as `api.js`:

- `fetchSnapshot(): Promise<Snapshot>` — sends `If-None-Match: snapshotEtag` if set; on 200 sets `snapshotEtag` from `ETag` header and `lastSnapshot`; on 304 returns `lastSnapshot` or throws `"HTTP 304 without cached snapshot"`; non-304 errors throw.
- `subscribeSnapshot(onSnapshot, onError): () => void` — opens `EventSource("/api/snapshot/stream")`, listens `"snapshot"` event (`JSON.parse` in try/catch, silent on failure), `onerror` → `es.close()` + `onError(Error)`. If `typeof EventSource === "undefined"` → immediate `onError` + no-op close.
- `transcribeAudio(form)` — POST `/api/transcribe`; `{res, data}` (`data` = parsed JSON or `{}`).
- `uploadClipboardImage(form, peer?)`, `uploadPaneFiles(form, peer?)` — POST with `peerQuery(peer)` (`?peer=encodeURIComponent(peer)` or `""`); `{res, data}`.
- `loadSTTConfig()`, `loadVersion()` — GET with `cache: "no-store"`; `{res, data}`.
- `saveSTTConfig({model, endpoint, api_key})` — PUT JSON; `{res, data}`.
- `loadAgentUsage()` — GET `/api/agent-usage` `cache: "no-store"`; caller inspects `res.status === 404`.

All wrappers preserve the shallow `{res, data}` envelope, the exact error strings, and the no-timeout fetch behavior.

### 5.2 WS/stream hook — `ws/usePaneStream.ts` (mirrors `stream.js`)

Closure-backed object exposed through a hook, preserving module-scoped semantics via refs (`ws`, `wsRetry`, `backoff`, `stableTimer`, `currentPane`):

- `open(paneID)`: if `paneID !== currentPane` → `backoff = 1000` (BEFORE socket creation); `status("connecting")`; `new WebSocket(wss|ws + "/ws/pane?pane=" + encodeURIComponent(paneID))`.
- `onopen` → `status("open")`, start `STABLE_MS = 5000` timer to reset backoff.
- `onmessage` → `JSON.parse` (silent on failure); `patch` → `onPatch(from|0, Array.isArray(lines)?lines:[], q ?? null)`; `error` → `onError(s)`.
- `onclose` → reconnect ONLY if `ws === sock && currentPane === paneID && !document.hidden`; capture `delay = backoff`; `backoff = Math.min(backoff*2, 30000)` (AFTER capture); `status("reconnecting")`; `setTimeout(reconnect, delay)`.
- `send(obj)`: `ws && ws.readyState === WebSocket.OPEN` → JSON-send, return `true`; else `false`.
- `close()`: cancel timers, null `ws`/`currentPane`, `onQuestion(null)`, `status("closed")`.

Hook responsibilities (from `app.js`): seed `paneLines` from `paneCache[paneID]` on open; apply patches `paneLines = paneLines.slice(0, from).concat(lines)`; call `setContent`; update `paneCache`; drive `ConnStatus`.

### 5.3 Typed store/context — `store/AppStateContext.tsx` (mirrors `state.js`)

Provides `state`, `voice`, `upload` as **mutable refs** plus a subscription/version-bump for re-render (the original mutates plain objects by reference). Shape exactly per `state.js`:

```ts
state: { selected: string | null; snapshot: Snapshot | null;
         drafts: Record<string,string>; paneOrder: string[]; selectionMode: boolean; }
voice: { recorder, stream, chunks, busy, mimeType, canceled, timer, startedAt,
         confirmOnStop, hotkeyDown, hotkeyStopPending, pendingBlob,
         suppressInputUntil, suppressedDraftValue }
upload: { busy: boolean }
```
Plus module-scoped refs that live in App: `paneLines`, `paneCache`, `snapshotSSE`, `snapshotTimer`, `ctrlArmed`, `imagePress`, `errorTimer`, `selectionRestored`, `copyFlashUntil`. Provide `selectPane`, `wsSend`, `findPane(id)`, `getSelectedPeer()`, `showInputError`, `setInputStatus`, `setConnStatus`, `syncSelectionButton` as stable callbacks injected into hooks.

### 5.4 Feature hooks

- `useSnapshotStream()` — SSE-first, fallback to 1000 ms polling on error; `applySnapshot` (render statusline, freeze `paneOrder`, restore selection once, prune cache, `checkStale`); visibility lifecycle.
- `useVoice({showInputError, syncDraft})` — exact `voice.js` state machine, 700 ms suppression, 250 ms timer, iOS mimeType trust.
- `useUpload({setInputStatus, showInputError, syncDraft, wsSend, getSelectedPeer})` — module-scoped `imgUploading`, `upload.busy`.
- `useUsage()` — 60 s poll, 404 stop+hide, local countdown recompute each render.
- `useViewport()` — `--tmact-vvh`, `scheduleFitViewport` (0/rAF/80/260 ms), `positionRecOverlay` on every fit.
- `useSettings()` / `useQuick()` / `useHotkeys()` — settings persistence + CSS-var/data-attr application; quick-button config + FAB; Option+key + Cmd-K/Ctrl-L hotkeys (capture phase, before DirectInput keydown).

---

## 6. Parity checklist (acceptance tests)

**Snapshot / polling / SSE**
1. On startup, fetch+apply snapshot, then subscribe SSE; on SSE error, fall back to 1000 ms polling with no dead window (start polling synchronously in `onError`).
2. `fetchSnapshot` sends `If-None-Match` when an ETag is cached; returns `lastSnapshot` on 304; throws `"HTTP 304 without cached snapshot"` if 304 with no cache.
3. Stale dot shows when `Date.now() - new Date(snap.ts).getTime() > 10000`; hidden otherwise; `Invalid Date` (missing `ts`) does not crash.
4. `restoreSelection` runs exactly once (first snapshot): exact `pane_id` match, else session-name match; guarded by `selectionRestored`.
5. `pruneCache` removes `paneCache` entries for deleted panes but keeps the selected pane.
6. On `visibilitychange` hidden: stop polling, close SSE, close WS. On visible: refresh snapshot, restart SSE, reopen WS for `state.selected`.

**Chips / selection**
7. Chips sorted peer → sessionLabel → window_index → pane_index; duplicate `baseLabel+session` keys append `:window_index`.
8. `state.paneOrder` frozen per `renderStatusline` so Option+key indices stay stable across snapshots.
9. Chip shows peer badge only if peer present; runtime icon cc/cx/cp/g (else dot `?` for asking-without-runtime); `.sel`/`.stale` classes; title = peer · cwd/session · state · hotkey.
10. Clicking a chip selects pane; re-selecting current pane reopens WS (reconnect) without closing first, preserving cached output.
11. Desktop non-selection-mode pane select auto-focuses `#direct-input`; mobile/selection-mode does not.

**Pane WS / patches**
12. First patch `from:0` replaces entire `paneLines`; subsequent splice `paneLines.slice(0,from).concat(lines)`; empty `lines` with `from = len` is a no-op append.
13. WS URL uses `wss:` on https, `ws:` on http; `pane` percent-encoded.
14. Backoff: first reconnect delay = 1000 ms; doubles AFTER capturing delay; caps at 30000; resets to 1000 after 5000 ms stable OR on pane switch (reset BEFORE socket creation).
15. Reconnect skipped if `document.hidden`, `currentPane` changed, or `ws !== sock` (stale close).
16. Connection-status strip shows `connecting…`/`reconnecting…`, cleared on `open`; never reflows chip list.
17. Malformed WS JSON silently dropped; `{t:"error"}` shown but does NOT close the socket.
18. `wsSend` returns `false` when not OPEN; all callers check and surface `"not connected — try again"`.

**Terminal rendering (terminal.js)**
19. ANSI SGR: 16-color palette, 256-color, RGB (38/48 variable-length subcodes; malformed dropped), bold/dim(opacity 0.6)/italic/underline/reverse(swaps fg/bg with `var(--bg)`/`var(--fg)`)/reset; trailing ANSI re-emitted.
20. HTML escaping (`&<>` only) happens BEFORE styled spans.
21. URL detection across soft-wraps; strips mid-URL ANSI and re-emits after placeholder; strips trailing `[.,;:!?)]}>'`]+`; `<a target="_blank" rel="noopener noreferrer">`.
22. Box-art tables: `joinWrappedFrames` merges via `FRAME_CONT_RE`; column count = (#`┬`/`╦`)+1; cell-count mismatch rejects whole block (raw fallback); `<thead>` only if `├─┤` separator present; bottom-frame scan capped at 600 lines; nested tables rejected.
23. Horizontal rules: ≥8 chars of `─`/`-` (ANSI-stripped) → `<span class="tui-rule">`.
24. Image paths: `IMAGE_PATH_RE` (.png/.jpg/.jpeg/.gif/.webp/.bmp/.svg, case-insensitive); not previewable if `~/`-prefixed; TreeWalker collects all text nodes before replacing; `<span class="image-path" data-path data-cwd data-peer>`.
25. Placeholder replacement order: rules → tables → URLs (private-use U+E000-range markers survive `escapeHTML`).
26. Auto-scroll: if within 60 px of bottom before render, scroll to bottom after; otherwise preserve.
26a. **Intentional deviation (web-UI improvement, NOT in the original):** before building HTML, `render()` strips trailing blank ROWS via `TRAILING_BLANK_RE` (a row is blank when it holds only whitespace and/or ANSI escapes — tmux `capture-pane -e` re-asserts SGR on empty cells, so padding can arrive as `\x1b[49m   `). tmux returns the full pane grid, so an idle prompt arrives as a few real lines + dozens of empty rows; rendering them verbatim made `#content` overflow and the stick-to-bottom auto-scroll (item 26) park on the blank tail, hiding the real prompt. Only trailing blank rows are removed — the last non-blank line keeps its own trailing spaces/SGR, and leading/interior blanks are untouched. The original `setContent` did NOT trim.

**Draft input**
27. `autoGrowDraft`: set `height:auto`, read `scrollHeight`+borders, clamp to 200 px, set `overflowY`.
28. Ctrl/Cmd+Enter sends `{t:"send",s}` and clears draft + `state.drafts[selected]`; Shift+Enter sends `{t:"text",s:"\n"}`; empty Enter (non-selection-mode) sends `{t:"key",k:"Enter"}` and switches to direct mode.
29. IME guard: `e.isComposing` and `keyCode===229` skip keydown relay.
30. `pointerdown` `preventDefault` on send/record/upload/selection/clear/draft-clear/FAB/option/copyline buttons keeps soft keyboard up (every one matters).
31. Image paste in draft inserts server path via `placeInDraft`; normal text paste falls through to textarea default.
32. Draft placeholder responsive: mobile vs desktop text.

**Direct mode**
33. `translateKey`: `metaKey` always → `null`; `Ctrl+<a-z>` → `C-<letter>`; Shift+Enter → newline; Tab/BTab/arrows/Home/End/PgUp/PgDn via KEYMAP; everything else printable → text.
34. `sendDirect` folds armed Ctrl into a single `C-<letter>` message; `ctrlArmed` auto-disarms after one key.
35. Soft-keyboard `input` event relays `direct.value` then clears it (overlay stays invisible).
36. `compositionend` sends `{t:"text",s:e.data}`.
37. Direct paste: image → upload + `sendDirect({t:"text",s:path+" "})` (trailing space); else text relay.

**Hotkeys**
38. Option+`Digit1…0`/`KeyQ…KeyP` selects `state.paneOrder[idx]`; desktop only (`isMobile()` skips); skipped if settings overlay open; `idx < paneOrder.length`.
39. Cmd+K (mac) / Ctrl+L (both) clears pane via `{t:"clear"}`; only if selected and settings closed.
40. Hotkey listener is capture-phase and ordered before `#direct-input` keydown.

**Key bar (mobile)**
41. Buttons for Esc/^C/Tab/⇧Tab/Ctrl-toggle/arrows/Home/End/PgUp/PgDn send `{t:"key",k}`; Ctrl toggle sticky, folds next key to `C-<letter>`, auto-disarms.
42. `syncKeyBar` clips to one row (`firstBtn.offsetHeight`, `scrollHeight > rowH+2`); expand toggle shows `⌃`/`⌄`; re-runs on resize.

**Selection mode / copy**
43. `toggleSelectionMode` sets `state.selectionMode`; when true, `#direct-input` blurred, `#content` `user-select:text` (desktop).
44. `mouseup` in `#content` skips refocus if non-collapsed selection with both `anchorNode`/`focusNode` in `#content`.
45. Cmd+click on `#content` image-path opens preview (separate from selection mode).
46. Copy-line bar visible when non-empty in-`#content` selection OR `Date.now() < copyFlashUntil` (900 ms flash).
47. `joinGlue` `/[ \t]*\n[ \t]*/g → ""`; `joinSpace` → `" "`.
48. `copyText`: `navigator.clipboard` if `window.isSecureContext`, else hidden-textarea + `execCommand("copy")` at `position:fixed;top:-1000px`.

**Image preview**
49. Long-press 550 ms with ≤10 px move; `pointerType==="mouse"` skips long-press; pointer move >10 px / pointerup / pointercancel clears timer.
50. Preview src `/api/image?path=&cwd=&peer=`; close on backdrop/Escape/close-button; closing clears `<img>.src`.

**Quick buttons**
51. `applicableQuick` = `common` + `RUNTIME_GROUP[runtime]` (claude/codex/shell), entries with empty text dropped; unknown runtime → common only.
52. Button click → `wsSend({t:"send",s:text})`; on success closes menu; on failure shows `"not connected — try again"` and keeps menu open.
53. FAB `.ready` when selected; `.open` toggles menu + backdrop; Escape closes only if open; backdrop click (`e.target===backdrop`) closes.
54. Editor (`#qb-editor`) groups common/claude/codex/shell; live `localStorage["tmact.quickButtons"]` save + menu re-render per keystroke; add/delete rows.
55. Config seeded from `QB_DEFAULT` on first run / invalid JSON; all 4 groups always present.

**Option bar**
56. `renderOptions(q)`: button per choice `[number] label`; click → `{t:"text",s:String(number)}`; cleared if `q` falsy or `choices` not array.

**Voice**
57. Record button disabled when no pane / `voice.busy` / `pendingBlob` / unsupported; title reflects state.
58. `preferredAudioType` order webm-opus → webm → mp4 → wav; iOS `isTypeSupported` undefined safe; `voice.mimeType` trusted over `recorder.mimeType`.
59. Button record auto-uploads on stop; Option+V hotkey (`altKey && !ctrl && !meta && code==="KeyV"`) records with confirm flow (V send / C cancel / Escape).
60. 700 ms `suppressInputUntil` blocks `beforeinput`/`input` (capture phase) on draft/direct to prevent `v` leak; draft restored to `suppressedDraftValue`.
61. `hotkeyStopPending` race: release before recorder ready → stop fired after `recorder.start()`.
62. Timer 250 ms, `M:SS`; overlay state classes transcribing/confirming/hotkey-recording; stop button pinned over record button; rec-card positioned above input bar.
63. `insertTranscript`: at-cursor if draft focused+selection, else replace-if-empty, else append with `\n`; empty transcript → `"empty transcript"` error; updates `state.drafts[selected]` before `syncDraft`.
64. Hotkey disabled on mobile and when settings overlay open; `window.blur` stops hotkey recording; stream tracks all stopped in `onstop`.

**Upload**
65. `pasteImage` guarded by `imgUploading`; FormData field `image`, filename `file.name || "paste.png"`; `?peer=` if selected peer; throws if `!res.ok` or no `data.path`; status `""` on success.
66. `uploadFilesToPane` guarded by `upload.busy` + `state.selected`; field `file` with fallback names `upload-1`…; coerces `data.paths || (data.path?[data.path]:[])`; sends `{t:"text",s:paths.join(" ")+" "}` (trailing space); error `"uploaded, but pane is not connected"` if `wsSend` false; `#upload-btn` disabled during upload then restored to `!state.selected`.
67. `openFileUploadPicker`: clear `input.value` first, try `showPicker()` else `click()`, errors via `showInputError`.
68. `placeInDraft`: at-cursor / replace-empty / append-with-space; returns early if `draft.disabled`; refocuses draft.

**Settings**
69. Open on gear; close on close-button / backdrop (`mousedown` + `e.target===overlay`) / Escape (only if `!hidden`); STT + version reloaded every open.
70. Font: `applyPaneFont` clamps 9–22, sets `--pane-font` on `<html>`, syncs slider/readout, persists; ±1 reads `getComputedStyle` `--pane-font`; default 13.
71. Running effect: normalize to RUNNING_EFFECTS (default `shine`), set `data-running-effect` on `<html>`, persist; preview animates 4 icons.
72. STT load: `#stt-key` always blank; note from `data.configured`; STT save PUT with blank-key-keeps-existing; status `saving…`→`saved ✓`(ok, auto-clear 4 s)/error(persist); button disabled during save.
73. `loadClientSettings` runs synchronously before first paint (apply font + effect).

**Usage**
74. 60 s poll + immediate first fetch; 404 → `clearInterval` + hide; transient errors keep last render; fetch exceptions suppressed.
75. Countdown recomputed from `Date.now()` each render (`fmtCountdown`/`fmtShort`); 59m→`0h59m`, 23h→`0d23h`, ≤0→`now`; missing `resets_at` → `""`.
76. `paceInfo` reserve = `-delta_percent`; `≥0` reserve/green `+NN%`, `<0` deficit/red `-NN%`; null pace → empty cell.
77. Grid: icon spans 2 rows normally (`u-icon-tall`), 1 row on error (`u-err` spans cols 2–4); missing window still renders 3 empty cells.

**Help**
78. Toggle via `#help-btn` (`pointerdown` preventDefault); close on Escape / overlay backdrop click; `body.help-open` class.
79. Tips filtered by `skip()` (mobile/desktop/disabled/missing element); rings before tips; banner first; `scoreCoachmark` weights (top 1×, left 0.35×, ring overlap 18×, card overlap 120×, 8 px insets, large-element 16% threshold); repositions on resize; uses `visualViewport`.

**Viewport / PWA**
80. `--tmact-vvh` = `visualViewport.height`; `scheduleFitViewport` at 0/rAF/80 ms/260 ms; `window.scrollTo(0,0)`; runs on visualViewport resize/scroll, orientationchange, focusin.
81. SW registers on `load`, errors ignored; network-first app-shell cache; `CACHE_NAME` hash flips when any embedded asset changes (verify via `/api/version` `asset_hash` matching SW cache name).
82. Manifest icons 180/192/512 present; 512 maskable.

**Error/status strips**
83. `showInputError` 6 s auto-clear via `errorTimer`; `setInputStatus` cancels `errorTimer` (no auto-clear); `setConnStatus` toggles `.show`, never reflows chips; `syncIndicator` hides strip only when both mode-text and input-error empty.

---

## 7. Migration work units (parallelizable)

Each unit is independently implementable once Unit 0–6 land.

| # | Unit | Output files |
|---|---|---|
| 0 | **Scaffold + build** — Vite/TS config, `index.html`, `public/sw.js` (verbatim `vDEV` token + `/assets/` prefix-cache), `manifest.json`, icons, `app.css` 1:1 port, Makefile `generate` → `outDir ../static`, SW registration. | `frontend/{vite.config.ts,tsconfig.json,index.html,package.json}`, `frontend/public/*`, `src/main.tsx`, `src/app.css` |
| 1 | **Server types** | `src/types/server.ts` |
| 2 | **API client** | `src/api/client.ts` |
| 3 | **Store/context + dom lib + keymap** | `src/store/AppStateContext.tsx`, `src/lib/dom.ts`, `src/lib/keymap.ts` |
| 4 | **Pane WS hook** | `src/ws/usePaneStream.ts` |
| 5 | **Snapshot stream hook** | `src/ws/useSnapshotStream.ts` |
| 6 | **Terminal renderer (pure)** | `src/terminal/render.ts` + unit tests |
| 7 | **StatusLine/Chip/ConnStatus/StaleDot/OptionBar** | `src/components/StatusLine.tsx,Chip.tsx,ConnStatus.tsx,OptionBar.tsx` |
| 8 | **ContentPane + CopyLineBar + ImagePreview** | `src/components/ContentPane.tsx,CopyLineBar.tsx,ImagePreview.tsx` |
| 9 | **InputBar: Draft + DirectInput + ModeIndicator** | `src/components/InputBar.tsx,Draft.tsx,DirectInput.tsx` |
| 10 | **KeyBar + useHotkeys** | `src/components/KeyBar.tsx`, `src/hooks/useHotkeys.ts` |
| 11 | **QuickDock + QuickEditor + useQuick** | `src/components/QuickDock.tsx,QuickEditor.tsx`, `src/hooks/useQuick.ts` |
| 12 | **Voice: RecOverlay + useVoice** | `src/components/RecOverlay.tsx`, `src/hooks/useVoice.ts` |
| 13 | **Upload hook + buttons + file input** | `src/hooks/useUpload.ts`, upload/clear/selection buttons in App |
| 14 | **UsagePanel + useUsage** | `src/components/UsagePanel.tsx`, `src/hooks/useUsage.ts` |
| 15 | **SettingsDialog + useSettings** | `src/components/SettingsDialog.tsx`, `src/hooks/useSettings.ts` |
| 16 | **HelpOverlay** | `src/components/HelpOverlay.tsx` |
| 17 | **useViewport** | `src/hooks/useViewport.ts` |
| 18 | **App orchestration** — wire all hooks/components, lifecycle, focus, error strips | `src/components/App.tsx` |

---

## 8. Risks & sequencing

### Build-first invariants (must land before anything renders)
1. **Unit 0** scaffold + the `app.css` 1:1 port. CSS is load-bearing for parity (`--pane-font`, `--tmact-vvh`, `data-running-effect`, `@keyframes agent-*`, `prefers-reduced-motion`, `760px`/`pointer:fine`/`any-pointer:coarse` media queries, safe-area insets). Do NOT translate to CSS-in-JS.
2. **Embed compatibility:** verify `vite build` writes `static/index.html`, `static/sw.js`, `static/manifest.json`, `static/icons/*` at root and hashed JS/CSS under `static/assets/`; confirm `go build` embeds them and `http.FileServer` serves the SPA. Confirm `/sw.js` still contains `tmact-app-shell-vDEV` so the Go regex rewrite fires; confirm changing any asset flips `/api/version` `asset_hash` and the SW `CACHE_NAME` suffix.
3. **Units 1–3** (types, API client, store/context) gate every component. Units 4–6 (WS, snapshot, terminal renderer) gate the content path. Everything ≥7 is parallelizable after that. Unit 18 (App) is last (integration).

### Highest-parity-risk areas (extra scrutiny + dedicated tests)
- **Imperative DOM where React batching would diverge:** `autoGrowDraft` needs synchronous `scrollHeight` reads (`useLayoutEffect`); `setContent` replaces whole `innerHTML` (imperative `pre#content.innerHTML` assignment + `markImagePaths`/scroll in a layout effect, NOT React reconciliation); focus management via refs.
- **`pointerdown` `preventDefault` on every button** (send/record/upload/selection/clear/draft-clear/FAB/option/copyline/help). Missing one breaks mobile soft-keyboard. Shared `onPointerDownNoBlur` + audit every button.
- **Listener phase/order:** hotkeys + voice suppression are capture-phase and must register before `#direct-input`'s keydown.
- **Module-scoped mutable state** (`backoff`, `currentPane`, `imgUploading`, `ctrlArmed`, `snapshotEtag`/`lastSnapshot`, `paneLines`/`paneCache`) → refs, not React state.
- **Timing constants:** `POLL_MS 1000`, `STALE_MS 10000`, `BACKOFF_MIN 1000`/`MAX 30000`/`STABLE 5000`, `IMAGE_LONG_PRESS_MS 550`/`MOVE 10`, voice timer `250`, suppression `700`, copy flash `900`, error clear `6000`, STT status clear `4000`, usage poll `60000`, viewport `80/260` ms.
- **WS atomicity:** `close()`-then-`open()` on re-select; `ws !== sock` stale-close guard; backoff reset BEFORE socket creation but doubling AFTER delay capture.

### Verification strategy
1. **Unit tests (Vitest):** port `terminal.js __test__` cases verbatim (tables, URLs, ANSI); test `translateKey`, client key gating, `joinGlue`/`joinSpace`, `fmtCountdown`/`fmtShort`, `paceInfo`, backoff sequencing, filename/peer-query helpers.
2. **Contract tests:** mock the frozen server (§2) and assert exact request shapes (FormData field names `audio`/`image`/`file`, `?peer=`, PUT blank-key semantics, `If-None-Match`) and exact client error strings.
3. **Go integration:** keep all existing `internal/web/*_test.go` green; add a build smoke check that `go build` embeds the Vite output and `/`, `/sw.js`, `/api/version` respond.
4. **Live-tmux smoke (`docs/smoke-test.md`):** desktop (Option+key, Cmd/Ctrl+Enter, direct mode, Cmd+click image, Cmd+K clear, copy bar) and mobile/iOS Safari (soft keyboard stays up on all buttons, `--tmact-vvh` fit, long-press preview, FAB quick send, voice confirm flow, key bar overflow). Verify SW cache-busting by editing one asset and confirming the new `CACHE_NAME` hash and a clean reload.
5. **Side-by-side parity diffing:** run old shell and new build against the same statusd; compare chip order/labels, rendered pane HTML (ANSI/tables/URLs), and scroll behavior on identical pane output.
