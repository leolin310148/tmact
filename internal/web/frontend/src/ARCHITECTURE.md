# ARCHITECTURE — tmact web frontend React port (coordination contract)

**This document governs every component/hook port (Units 7–18). Conform to it
exactly.** It is the binding contract that lets parallel agents implement
pieces that wire together without re-reading each other's code. Where this
document and MIGRATION_SPEC.md agree, MIGRATION_SPEC.md is the authoritative
parity spec for *behavior*; THIS doc is authoritative for *how the React pieces
connect* (context API, callback signatures, conventions, wiring order).

The original is a faithful 1:1 behavioral port — preserve exact timing
constants, WS/HTTP message shapes, error strings, localStorage keys, DOM ids,
and class names. When in doubt, match the original byte-for-behavior. Do NOT
refactor semantics or "improve".

---

## 0. The golden rules (every file)

1. **State that the original mutated by reference lives in refs, mutated in
   place — never useState, never cloned.** The store (`AppStateContext.tsx`)
   already wraps `state` / `voice` / `upload` this way. Other module-scoped
   mutable values (`paneLines`, `paneCache`, `snapshotSSE`, `snapshotTimer`,
   `ctrlArmed`, `imagePress`, `errorTimer`, `selectionRestored`,
   `copyFlashUntil`, stream's `ws`/`backoff`/`currentPane`, api's
   `snapshotEtag`/`lastSnapshot`, upload's `imgUploading`) are likewise `useRef`
   (or module-level `let` in non-hook modules like `api/client.ts`).
2. **Re-render is explicit.** After mutating store state where the original
   re-ran a render function, call `bump()` from the store. Components subscribe
   by calling `useAppState()`.
3. **Pane output HTML is set imperatively, never reconciled.** `ContentPane`
   sets `pre#content`'s inner HTML via the pure renderer + `dangerouslySetInnerHTML`
   (or a direct `innerHTML` assignment in a layout effect), then runs
   `markImagePaths` and scroll restoration in a `useLayoutEffect`. React must
   never diff the pane's children.
4. **`pointerdown` `preventDefault` on every interactive button**, via the
   shared `onPointerDownNoBlur` helper (`lib/dom.ts`). Missing one breaks the
   mobile soft keyboard. Audit list in §6.
5. **Capture-phase listeners (hotkeys + voice suppression) register BEFORE the
   `#direct-input` keydown handler.** See §7 wiring order.
6. **Keep DOM ids and class names IDENTICAL to the original** (the CSS in
   `app.css` is verbatim and selects on them). Element ids are listed in §8.
7. **TypeScript strict + `noUncheckedIndexedAccess`.** Indexed access yields
   `T | undefined`; narrow it. No `any`. Imports omit extensions
   (bundler resolution).

---

## 1. AppStateContext API (Unit 3 — DONE)

`src/store/AppStateContext.tsx` exports:

```ts
// Shapes (field-for-field with state.js):
interface AppState   { selected: string | null; snapshot: Snapshot | null;
                       drafts: Record<string,string>; paneOrder: string[];
                       selectionMode: boolean; }
interface VoiceState { recorder, stream, chunks, busy, mimeType, canceled,
                       timer, startedAt, confirmOnStop, hotkeyDown,
                       hotkeyStopPending, pendingBlob, suppressInputUntil,
                       suppressedDraftValue }
interface UploadState { busy: boolean }

interface AppCallbacks { selectPane, wsSend, findPane, getSelectedPeer,
                         showInputError, setInputStatus, setConnStatus,
                         syncSelectionButton, syncDraft, openSettings,
                         closeSettings }   // signatures in §4

interface AppStateContextValue {
  state: AppState;            // mutate in place
  voice: VoiceState;          // mutate in place
  upload: UploadState;        // mutate in place
  bump: () => void;           // call after a mutation to re-render subscribers
  version: number;            // monotonic; for useMemo/useEffect deps
  callbacks: AppCallbacks;    // implemented by App
}

// App-side:
function useAppStateStore(): { value: AppStateContextValue;
                               setCallbacks: (cb: AppCallbacks) => void };
function AppStateProvider({ store, children }): JSX.Element;
// Consumer-side:
function useAppState(): AppStateContextValue;   // throws outside provider
```

### How a component subscribes / re-renders

```ts
const { state, voice, upload, bump, callbacks } = useAppState();
```

Calling `useAppState()` subscribes the component to `bump()`: every bump
increments `version`, which changes the memoized context value identity, which
re-renders all consumers. The object references inside (`state`, `voice`,
`upload`) are STABLE for the app's lifetime — they are mutated in place, so a
component reads `state.selected` etc. and gets the current value on each render.

**Read-then-bump pattern (mirrors the original render-after-mutate):**

```ts
state.selected = paneID;           // mutate by reference, exactly like app.js
state.drafts[paneID] = value;      // mutate nested by reference
bump();                            // where app.js called renderStatusline()/renderMode()/...
```

**Never** do `setState({ ...state, selected })`. There is no setState for these.

### Who creates the store

`App` (Unit 18) calls `useAppStateStore()` ONCE, renders
`<AppStateProvider store={store}>`, and calls `store.setCallbacks({...})` with
its wired callback implementations (from a layout effect or synchronously after
defining them — callbacks are read through a ref, so a post-first-render
`setCallbacks` is observed without a bump).

---

## 2. Module/file layout (import paths)

```
src/
  types/server.ts          — server contract types (DONE)
  lib/dom.ts               — isMobile, escapeHTML, clamp, onPointerDownNoBlur (DONE)
  lib/keymap.ts            — PANE_HOTKEYS, HOTKEY_CODE, HOTKEY_INDEX, KEYMAP, translateKey (DONE)
  api/client.ts            — fetchSnapshot, subscribeSnapshot, transcribeAudio,
                             uploadClipboardImage, uploadPaneFiles, loadSTTConfig,
                             loadVersion, loadAgentUsage, saveSTTConfig (DONE)
  store/AppStateContext.tsx — this contract (DONE)
  ws/usePaneStream.ts      — pane WS hook (Unit 4)
  ws/useSnapshotStream.ts  — snapshot SSE+poll hook (Unit 5)
  terminal/render.ts       — pure render(text, opts) + markImagePaths (Unit 6)
  hooks/                   — useVoice, useUpload, useUsage, useViewport, useSettings,
                             useQuick, useHotkeys, useHelp
  components/              — App, StatusLine, Chip, ConnStatus, OptionBar,
                             ContentPane, CopyLineBar, ImagePreview, InputBar, Draft,
                             DirectInput, KeyBar, ModeIndicator, QuickDock, QuickEditor,
                             UsagePanel, SettingsDialog, RecOverlay, HelpOverlay
```

**Relative import depth** (from this doc's mandate "Foundation import paths:
`../types/server`, `../lib/dom`, …, `../store/AppStateContext`"): components and
hooks live one level under `src/`, so they import:

```ts
import { useAppState } from "../store/AppStateContext";
import type { Snapshot, PaneStatus, InputMsg } from "../types/server";
import { isMobile, escapeHTML, clamp, onPointerDownNoBlur } from "../lib/dom";
import { translateKey, HOTKEY_INDEX } from "../lib/keymap";
import { fetchSnapshot } from "../api/client";
```

Omit file extensions (Vite bundler resolution; `moduleResolution: "Bundler"`).
Do NOT write `.ts`/`.tsx` in import specifiers.

---

## 3. Component vs. hook conventions

- **Components are presentational.** They read state via `useAppState()` and
  receive anything else they need via props. They contain JSX matching the
  original DOM (same ids/classes). They do NOT own cross-cutting mutable state
  (that lives in App or in feature hooks).
- **Feature hooks mirror the original factory modules** (voice.js / upload.js /
  quick.js / settings.js / usage.js / help.js / stream.js). Each takes a single
  **injected-deps object** matching the original `createX({...})` parameters, so
  the hook never reaches back into App by import. The deps come from
  `useAppState().callbacks` plus App-local helpers. Hooks return the same
  named functions the factory returned.
- A hook OWNS the imperative resources the original module owned (timers,
  MediaRecorder, WebSocket, EventSource) via refs, and cleans them up where the
  original did (visibility lifecycle, `close()`, etc.). Hooks do NOT register
  their own React state for these — refs only.

### Injected-deps contracts (match the factory params exactly)

```ts
// ws/usePaneStream.ts  ← createPaneStream(callbacks)  [stream.js]
usePaneStream({
  getSelectedPane: () => string | null,   // () => state.selected
  onPatch: (from: number, lines: string[], question: Question | null) => void,
  onQuestion: (q: Question | null) => void,
  onError: (msg: string) => void,
  onStatus: (s: "connecting" | "open" | "reconnecting" | "closed") => void,
}) => { open(paneID: string): void; close(): void; send(obj: InputMsg): boolean }

// hooks/useVoice.ts  ← createVoice({ showInputError, syncDraft })  [voice.js]
useVoice({ showInputError, syncDraft })
  => { syncRecordButton, positionRecOverlay, startRecording, stopRecording,
       cancelRecording, finishRecordingConfirm, wireRecordHotkey }

// hooks/useUpload.ts  ← createUpload({...})  [upload.js]
useUpload({ setInputStatus, showInputError, syncDraft, wsSend, getSelectedPeer })
  => { clipboardImage, pasteImage, uploadFilesToPane, openFileUploadPicker, placeInDraft }

// hooks/useQuick.ts  ← createQuick({...})  [quick.js]
useQuick({ wsSend, showInputError, findPane, syncSelectionButton })
  => { loadQuickConfig, wireQuick, syncQuickDock, closeQuickMenu }

// hooks/useSettings.ts  ← settings.js (no factory; module-level)
useSettings() => { loadClientSettings, openSettings, closeSettings, /* wireSettings via JSX */ }
//   localStorage key "tmact.settings"; FONT_MIN 9 / MAX 22 / DEFAULT 13;
//   RUNNING_EFFECTS ["shine","pulse","rainbow","scan","none"] default "shine".

// hooks/useUsage.ts  ← usage.js wireUsage()
useUsage() => void   // owns the 60 s poll; 404 → stop + hide; renders #usage-panel grid

// hooks/useViewport.ts  ← app.js fitViewport/scheduleFitViewport
useViewport({ positionRecOverlay })   // positionRecOverlay injected from useVoice

// hooks/useHotkeys.ts  ← app.js wireHotkeys
useHotkeys({ selectPane, clearPaneOutput, settingsOpen: () => boolean })

// hooks/useHelp.ts  ← help.js wireHelp/placeCoachmarks
useHelp()   // reads state for skip() predicates
```

`wsSend`, `showInputError`, `setInputStatus`, `setConnStatus`,
`syncSelectionButton`, `syncDraft`, `findPane`, `getSelectedPeer` all come from
`useAppState().callbacks` (App implements them). `selectPane` likewise.

---

## 4. Shared callback contract (signatures + who implements/consumes)

All implemented by **App** (Unit 18) and provided through
`AppStateContextValue.callbacks`. Hooks/components consume them.

| Callback | Signature | Implements | Consumed by | Behavior (must match app.js) |
|---|---|---|---|---|
| `selectPane` | `(paneID: string) => void` | App | Chip onclick, useHotkeys, useSnapshotStream.restoreSelection | No-op if falsy id. Re-selecting current pane → `openWS` (reconnect) keeping cached output. Else: set `state.selected`, persist `{pane,session}` to `localStorage["tmact.selectedPane"]`, restore draft, enable draft/send/upload, `syncDraft`/`syncRecordButton`/`syncSelectionButton`, `setContent("Loading…")`, re-render statusline + mode, `closeQuickMenu`, `syncQuickDock`, scroll `.chip.sel` into view, `openWS`. Desktop non-selection-mode → focus `#direct-input`. |
| `wsSend` | `(msg: InputMsg) => boolean` | App (→ usePaneStream.send) | useVoice (none), useUpload, useQuick, OptionBar, KeyBar, DirectInput, Draft, clearPaneOutput | `true` if socket OPEN & sent; else `false`. **Every caller checks `false` and calls `showInputError("not connected — try again")`** (except where the original used a different string — useUpload's file path uses `"uploaded, but pane is not connected"`). |
| `findPane` | `(paneID: string \| null) => PaneStatus \| null` | App | useQuick, useUpload (via getSelectedPeer), ContentPane | Scans `state.snapshot.panes` for `pane_id === paneID`; null if snapshot/id/pane absent. |
| `getSelectedPeer` | `() => string` | App | useUpload | `panePeer(findPane(state.selected))` → `p.peer ? String(p.peer) : ""`. |
| `showInputError` | `(msg: string) => void` | App | nearly everything | Sets `#input-error` text, `syncIndicator`, clears+sets a 6000 ms `errorTimer` that empties it. |
| `setInputStatus` | `(msg: string) => void` | App | useUpload | Cancels `errorTimer` (no auto-clear), sets `#input-error` text, `syncIndicator`. Cleared by `""`. |
| `setConnStatus` | `(msg: string) => void` | App | usePaneStream onStatus (via App) | Sets `#conn-status` text, toggles `.show` (`msg !== ""`). Never reflows chips. |
| `syncSelectionButton` | `() => void` | App | useQuick.syncQuickDock, selectPane, toggleSelectionMode | Toggles `#selection-btn` `.ready`(selected) / `.active`(selectionMode); `disabled = !selected`; `aria-pressed`; `title`. |
| `syncDraft` | `() => void` | App | useVoice, useUpload, selectPane, clearDraft, sendDraft, input handlers | Toggle `#draft-wrap.has-text` (`!disabled && value!==""`); `autoGrowDraft()` (height:auto → scrollHeight+borders, cap 200 px, set overflowY). Needs `useLayoutEffect` semantics (synchronous scrollHeight). |
| `openSettings` | `() => void` | App (→ useSettings) | GearButton, HelpOverlay (none) | `#settings-overlay.hidden = false`; reload STT + version. |
| `closeSettings` | `() => void` | App (→ useSettings) | SettingsDialog close/backdrop/Escape | `#settings-overlay.hidden = true`. |

**App-local (not in the context contract, but App must own and pass down):**
`openWS(paneID)` / `closeWS()` (seed from `paneCache`, then `paneStream.open`),
`clearPaneOutput()` (`wsSend({t:"clear"})` else error), `renderStatusline` /
`renderMode` / `syncIndicator` / `checkStale` (these become React renders driven
by `bump()` rather than imperative DOM rebuilds), `applySnapshot`,
`restoreSelection`, `pruneCache`, `rememberSelection`, `toggleSelectionMode`,
`clearDraft`, `sendDraft`, `autoGrowDraft`, `syncQuickDock`/`closeQuickMenu`
(from useQuick), `syncRecordButton` (from useVoice).

### WS / HTTP message shapes (frozen — do not change)

- Click a quick-answer choice: `wsSend({ t: "text", s: String(choice.number) })`.
- Quick-button: `wsSend({ t: "send", s: text })`.
- Draft send (Ctrl/Cmd+Enter or Send): `wsSend({ t: "send", s: draft.value })`.
- Shift+Enter in direct: `{ t: "text", s: "\n" }`. Empty-Enter in draft →
  direct mode + `{ t: "key", k: "Enter" }`.
- Direct keystrokes: `translateKey(e)` from `lib/keymap` (already ported).
- Helper key bar: `{ t: "key", k }`; sticky Ctrl folds next text key to `C-<letter>`.
- Clear pane (Cmd+K / Ctrl+L / clear button): `{ t: "clear" }`.
- File upload result relay: `{ t: "text", s: paths.join(" ") + " " }` (trailing space).
- Image paste (draft): path inserted via `placeInDraft`. Image paste (direct):
  `sendDirect({ t: "text", s: path + " " })` (trailing space).

### Error strings (verbatim)

- `"not connected — try again"` (most send failures)
- `"uploaded, but pane is not connected"` (file upload, wsSend false)
- `"empty transcript"`, `"no audio recorded"`, `"transcription failed"`,
  `"microphone recording is not supported in this browser"`,
  `"microphone permission denied"`, `"microphone unavailable"`,
  `"microphone recording failed"`
- `"select a pane first"` (file upload, no selection)
- `"image upload failed"`, `"file upload failed"`, `"file picker blocked"`
- `"HTTP 304 without cached snapshot"` (api/client — already ported)

---

## 5. localStorage keys / formats (verbatim)

| Key | Value | Owner |
|---|---|---|
| `tmact.selectedPane` | `JSON.stringify({ pane: <id>, session: <name> })` | App `rememberSelection`/`restoreSelection` |
| `tmact.settings` | `JSON.stringify({ paneFont?: number, runningEffect?: string })` | useSettings |
| `tmact.quickButtons` | `JSON.stringify({ common:[], claude:[], codex:[], shell:[] })` (each entry `{label,text}`) | useQuick |

All reads are try/catch-guarded and tolerate malformed/absent values exactly as
the originals (seed defaults / fall back to `{}`/`null`).

---

## 6. Buttons that MUST use `onPointerDownNoBlur` (`onPointerDown={onPointerDownNoBlur}`)

(`onPointerDownNoBlur` calls `e.preventDefault()`; from `lib/dom.ts`.)

- `#draft-clear`, `#send-btn`, `#record-btn`, `#clear-pane-btn`, `#selection-btn`
- `#upload-btn` (it has a click handler; original adds preventDefault on the
  group — match the original; if the original did not add it to upload-btn,
  do NOT add it — see app.js: upload-btn has NO pointerdown preventDefault. Only
  send/record/clear-pane/selection do. Audit per-button against app.js lines
  709–712 and the option/keybar/quick/help/copyline handlers.)
- Option-bar buttons (`#option-bar button`) — `pointerdown` preventDefault.
- Key-bar buttons + `#key-toggle` (`buildKeyBar`).
- `#qb-fab` (quick FAB).
- Quick-menu buttons (`renderQuickMenu`).
- `#help-btn` and `#help-overlay`.
- `#copyline-join`, `#copyline-space`, `#copyline-run`.

**Exact parity note:** the set of buttons that get `pointerdown` preventDefault
is load-bearing for the mobile soft keyboard (spec §6 item 30). Re-derive each
component's set from its original module — do NOT blanket-apply to every button.
In app.js `wireInput`: only `draft-clear`, `send-btn`, `record-btn`,
`clear-pane-btn`, `selection-btn` get it (NOT `upload-btn`). Option bar, key bar,
quick, help, copyline each add their own in their modules.

---

## 7. Pane output: imperative HTML rule

`terminal.js`'s `setContent(text, opts)` does `pre#content.innerHTML = html`
then `markImagePaths(pre, cwd, peer)` then auto-scroll. In React:

- `terminal/render.ts` (Unit 6) is a PURE function `render(text, opts) =>
  htmlString` (ANSI→HTML, URLs, tables, rules) plus `markImagePaths(preEl, cwd,
  peer)` which mutates an existing DOM node (TreeWalker pass).
- `ContentPane` (Unit 8) holds a `ref` to `pre#content`. In a `useLayoutEffect`
  keyed on the rendered text/opts: read `atBottom = scrollHeight - scrollTop -
  clientHeight < 60` BEFORE writing, set `pre.innerHTML = render(text, opts)` (or
  `dangerouslySetInnerHTML`), call `markImagePaths(pre, cwd, peer)`, then if
  `atBottom` set `pre.scrollTop = pre.scrollHeight`. React must NEVER receive
  the rendered HTML as JSX children — it is opaque imperative content.
- App owns `paneLines` (ref array) and `paneCache` (ref `Record<string,string[]>`):
  - `openWS`: `paneLines.current = cache[id]?.slice() ?? []`; if non-empty,
    `setContent(join, {cwd, peer})` immediately; then `paneStream.open(id)`.
  - `onPatch`: `paneLines.current = paneLines.current.slice(0, from).concat(lines)`;
    if selected, `paneCache.current[selected] = paneLines.current`; `setContent(...)`;
    `renderOptions(question)`.
  - "setContent" in React = update the state ContentPane reads, then its layout
    effect rewrites innerHTML. Provide the latest `{text, cwd, peer}` to
    ContentPane (e.g. via a ref + bump, or a small dedicated piece of state in
    App that ContentPane reads — but the INNER HTML write stays imperative).

---

## 8. DOM ids / class names (keep verbatim — CSS depends on them)

Element ids referenced by the original (and thus by ports): `root`, `chips`,
`conn-status`, `option-bar`, `mode-indicator`, `mode-text`,
`input-error`, `input-bar`, `content-wrap`, `content`, `direct-input`, `draft`,
`draft-wrap`, `draft-clear`, `send-btn`, `record-btn`, `clear-pane-btn`,
`upload-btn`, `selection-btn`, `file-upload`, `rec-overlay`, `rec-stop`,
`rec-send`, `rec-cancel`, `rec-label`, `rec-timer`, `key-area`, `key-bar`,
`key-toggle`, `ctrl-key`, `qb-dock`, `qb-fab`, `qb-menu`, `qb-backdrop`,
`qb-editor`, `gear-btn`, `settings-overlay`, `settings-close`, `font-range`,
`font-val`, `font-dec`, `font-inc`, `running-effect`, `stt-model`,
`stt-endpoint`, `stt-key`, `stt-note`, `stt-status`, `stt-save`, `build-time`,
`asset-hash`, `help-btn`, `help-overlay`, `usage-panel`, `copyline-bar`,
`copyline-join`, `copyline-space`, `statusline`.

Key class names: `chip`/`chip.sel`/`chip.stale`, `chip-key`, `peer-badge`,
`agent-icon`/`runtime-<name>`/`running`/`asking`/`stale`, `dot`, `empty`,
`tui-rule`, `tui-link`, `image-path`, `direct`, `selection-mode`,
`upload-ready`, `has-text`, `ready`/`active`/`open`/`busy`/`armed`/`expanded`/
`overflowing`/`show`/`visible`/`copied`, `image-preview*`, `help-ring`/
`help-tip`/`help-banner`/`tone-*`, `rec-card`, `u-icon`/`u-icon-tall`/`u-remain`/
`u-pace`/`reserve`/`deficit`/`u-time`/`u-err`, `qb-*`, `settings-status`,
`help-open` (on body).

`data-` attributes: `data-running-effect` (on `<html>`), `data-path`/`data-cwd`/
`data-peer` (on `.image-path`). CSS vars: `--pane-font`, `--tmact-vvh`.

---

## 9. App.tsx wiring order (Unit 18)

This order is load-bearing (mirrors app.js bottom block + capture-phase
constraints). App is the only place that knows the whole graph.

1. **Synchronous, before first paint:** `loadClientSettings()` (apply
   `--pane-font` + `data-running-effect` from `localStorage["tmact.settings"]`)
   and `loadQuickConfig()` (seed `tmact.quickButtons`). The original calls these
   first, synchronously — do them in a `useLayoutEffect` that runs before the
   first content render, or via a module-level call equivalent to app.js's
   top-level execution. They must take effect before the first snapshot render.
2. **Create the store** (`useAppStateStore`), wrap children in
   `AppStateProvider`, and **register callbacks** (`store.setCallbacks({...})`)
   with App's implementations of every `AppCallbacks` member.
3. **Instantiate the pane stream** (`usePaneStream({ getSelectedPane, onPatch,
   onQuestion, onError: showInputError, onStatus })`). `wsSend` = stream.send.
4. **Wire input handlers** (Draft/DirectInput/KeyBar/CopyLine via their
   components) — equivalent to `wireInput`/`wireCopyLine`.
5. **Register capture-phase listeners BEFORE direct-input keydown:**
   `useHotkeys` (Option+key, Cmd+K/Ctrl+L) and `useVoice`'s `wireRecordHotkey`
   (Option+V + 700 ms input suppression on `beforeinput`/`input`) both attach
   `document.addEventListener(..., true)` (capture). They MUST be registered
   before `#direct-input`'s keydown so the hotkey/suppression handlers win.
   Practically: mount the hotkey/voice hooks (which add capture listeners on
   mount) above/earlier than DirectInput's own keydown registration, or ensure
   DirectInput uses a non-capture (bubble) keydown so capture always precedes it
   (the original direct-input keydown is bubble-phase — keep it bubble).
6. **Settings / quick / help / usage / viewport** wiring (`wireSettings`,
   `wireQuick`, `wireHelp`, `wireUsage`, `wireRecordHotkey`, viewport listeners).
7. **SW registration** is in `main.tsx` already (on `window` `load`,
   fire-and-forget). Do not duplicate.
8. **Start snapshot delivery:** `refreshSnapshot()` once immediately, then if
   `!document.hidden` start the SSE stream (`useSnapshotStream` handles
   SSE-first → 1000 ms poll fallback, `applySnapshot`, freeze `paneOrder`,
   `restoreSelection` once, `pruneCache`, `checkStale`).
9. **Visibility lifecycle:** on `visibilitychange` hidden → stop polling, close
   SSE, close WS; on visible → `refreshSnapshot`, restart SSE, reopen WS for
   `state.selected`. (useSnapshotStream + App coordinate; WS reopen is App's.)

**Timing constants (do not drift):** `POLL_MS 1000`, `STALE_MS 10000`,
backoff `1000…30000` / stable `5000`, image long-press `550` / move `10`, voice
timer `250` / suppression `700`, copy flash `900`, error clear `6000`, STT
status clear `4000`, usage poll `60000`, viewport schedule `0/rAF/80/260` ms.

---

## 10. Re-render coordination summary

The original re-ran imperative render functions after mutations. In the port,
those become React renders triggered by `bump()`:

| Original imperative call | Port equivalent |
|---|---|
| `renderStatusline(snap)` | mutate `state.snapshot`/`state.selected`/`state.paneOrder`, `bump()`; `StatusLine` re-renders chips from `state.snapshot`. |
| `renderMode()` / `syncIndicator()` | `bump()`; `ModeIndicator`/`InputBar`/`ContentWrap` recompute `.direct`/`.selection-mode` classes from `state` + `document.activeElement`. |
| `syncDraft()` | imperative (`useLayoutEffect` on Draft) — synchronous scrollHeight; may also `bump()` for `.has-text`. |
| `checkStale()` | `ConnStatus` recomputes freshness from `state.snapshot.ts` and `stale_after_ms` on each render / a 1 s timer; stale snapshot delivery is shown in the same fixed top-center overlay as pane stream reconnects. |
| Missing selected local pane | When an authoritative snapshot no longer contains the selected local pane, App closes its WebSocket, clears selection/content/draft state and persistence, and disables pane-owned controls. Missing peer panes are retained because peer fetch failures can be transient. |
| `setContent(...)` | App updates ContentPane's text/opts; ContentPane's layout effect rewrites innerHTML imperatively. |
| `renderOptions(q)` | App stores latest question (ref/state); `OptionBar` renders buttons from it. |
| `setConnStatus` / `showInputError` / `setInputStatus` | App holds the strings in refs/state and `bump()`; `ConnStatus` / `ModeIndicator` render them (toggling `.show`). |

When a component needs the freshest `state` value mid-render, read the live
object (`state.selected`) — it is always current because it is mutated in place.
`version` only forces the re-render; it is not the data.
