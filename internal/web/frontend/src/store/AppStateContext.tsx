// Shared application state + callback contract — the React port of
// static/js/state.js plus the orchestration surface that static/app.js exposes
// to its factory modules (voice.js / upload.js / quick.js / etc.).
//
// PARITY MODEL (read ARCHITECTURE.md §1–§3 before touching this file):
//   The original app keeps `state`, `voice`, and `upload` as module-scoped
//   PLAIN OBJECTS that every module mutates BY REFERENCE (e.g.
//   `state.selected = paneID`, `state.drafts[id] = v`, `voice.busy = true`).
//   There is no reactive layer — the original simply re-runs imperative render
//   functions after a mutation. To preserve that exact mutation timing (and to
//   avoid the stale-closure traps of useState), we hold each object in a
//   `useRef` and mutate it by reference, IDENTICALLY to the original. React
//   re-renders are driven explicitly by `bump()` (a version counter), which the
//   orchestrator (App) and hooks call AFTER they mutate, mirroring where the
//   original called its render functions.
//
//   DO NOT convert these objects to useState. DO NOT spread/clone them on
//   mutation. Mutate in place, then call `bump()` where the original called a
//   render function (renderStatusline / renderMode / syncDraft / …).

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { Snapshot, InputMsg, PaneStatus } from "../types/server";

// ---------------------------------------------------------------------------
// state.js shapes — field-for-field, names verbatim
// ---------------------------------------------------------------------------

/** Mirrors `state` in state.js. Mutated by reference; never replaced. */
export interface AppState {
  /** Selected pane id (`%12` or `peer@%12`), or null when none. */
  selected: string | null;
  /** Last applied snapshot, or null before the first one lands. */
  snapshot: Snapshot | null;
  /** Per-pane draft textarea contents, keyed by pane id. */
  drafts: Record<string, string>;
  /** Frozen render order of pane ids (drives Option+key hotkey indices). */
  paneOrder: string[];
  /** Whether selection (copy) mode is on. */
  selectionMode: boolean;
}

/**
 * Mirrors `voice` in state.js. Holds live MediaRecorder/stream/timer handles
 * plus the hotkey/suppression bookkeeping. All fields are mutated in place by
 * useVoice; none are React state. Types are widened where the original stored
 * heterogeneous values (e.g. `timer` is a setInterval handle).
 */
export interface VoiceState {
  recorder: MediaRecorder | null;
  stream: MediaStream | null;
  chunks: Blob[];
  busy: boolean;
  mimeType: string;
  canceled: boolean;
  /** setInterval handle for the elapsed-time ticker, or null. */
  timer: ReturnType<typeof setInterval> | null;
  /** Live Web Audio handles used only to render the recording waveform. */
  audioContext: AudioContext | null;
  analyser: AnalyserNode | null;
  audioSource: MediaStreamAudioSourceNode | null;
  waveformRAF: number | null;
  waveformData: Uint8Array<ArrayBuffer> | null;
  startedAt: number;
  confirmOnStop: boolean;
  hotkeyDown: boolean;
  hotkeyStopPending: boolean;
  pendingBlob: Blob | null;
  /** Epoch ms until which draft/direct text input is suppressed (700 ms guard). */
  suppressInputUntil: number;
  /** Draft value snapshot restored after a suppressed `v` keystroke. */
  suppressedDraftValue: string | null;
}

/** Mirrors `upload` in state.js. */
export interface UploadState {
  busy: boolean;
}

// Initial values — byte-identical to state.js's literals.
function initialState(): AppState {
  return {
    selected: null,
    snapshot: null,
    drafts: {},
    paneOrder: [],
    selectionMode: false,
  };
}

function initialVoice(): VoiceState {
  return {
    recorder: null,
    stream: null,
    chunks: [],
    busy: false,
    mimeType: "",
    canceled: false,
    timer: null,
    audioContext: null,
    analyser: null,
    audioSource: null,
    waveformRAF: null,
    waveformData: null,
    startedAt: 0,
    confirmOnStop: false,
    hotkeyDown: false,
    hotkeyStopPending: false,
    pendingBlob: null,
    suppressInputUntil: 0,
    suppressedDraftValue: null,
  };
}

function initialUpload(): UploadState {
  return { busy: false };
}

// ---------------------------------------------------------------------------
// Shared callback contract — implemented by App, consumed by hooks/components.
// Signatures mirror the corresponding app.js functions EXACTLY. See
// ARCHITECTURE.md §4 for who implements vs. consumes each.
// ---------------------------------------------------------------------------

export interface AppCallbacks {
  /**
   * app.js `selectPane(paneID)`. Selects a pane (or, when re-selecting the
   * current one, forces a WS reconnect without blanking). No-op on a falsy id.
   * Drives draft restore, button enablement, statusline re-render, WS open, and
   * desktop direct-input focus.
   */
  selectPane: (paneID: string) => void;

  /**
   * app.js `wsSend(obj)` → `paneStream.send(obj)`. Returns `true` when the
   * socket was OPEN and the frame was sent, `false` otherwise. EVERY caller must
   * check the boolean and surface `"not connected — try again"` on `false`
   * (see ARCHITECTURE.md §4).
   */
  wsSend: (msg: InputMsg) => boolean;

  /**
   * app.js `findPane(paneID)`. Looks up a pane in the current snapshot by
   * `pane_id`; returns null when the snapshot, id, or pane is absent.
   */
  findPane: (paneID: string | null) => PaneStatus | null;

  /**
   * Mirrors app.js's inline `getSelectedPeer` closure passed to createUpload:
   * `() => panePeer(findPane(state.selected))`. Returns "" when no pane / no peer.
   */
  getSelectedPeer: () => string;

  /**
   * app.js `showInputError(msg)`. Shows a transient error in the input-bar
   * indicator slot (`#input-error`) and auto-clears after 6000 ms via the shared
   * `errorTimer`.
   */
  showInputError: (msg: string) => void;

  /**
   * app.js `setInputStatus(msg)`. Shows a persistent note in the same
   * `#input-error` slot (e.g. "uploading image…"); CANCELS the auto-clear timer.
   * Caller clears by passing "".
   */
  setInputStatus: (msg: string) => void;

  /**
   * app.js `setConnStatus(msg)`. Shows the live-connection note in the strip
   * above the chips (`#conn-status`), toggling `.show`. Empty string hides it.
   * Never reflows the chip row.
   */
  setConnStatus: (msg: string) => void;

  /**
   * app.js `syncSelectionButton()`. Reconciles `#selection-btn` classes
   * (`.ready`/`.active`), `disabled`, `aria-pressed`, and `title` from
   * `state.selected` + `state.selectionMode`.
   */
  syncSelectionButton: () => void;

  /**
   * app.js draft helper `syncDraft()`. Keeps the `#draft-wrap.has-text` clear
   * button and the textarea height in step with the draft contents; calls
   * `autoGrowDraft`. Injected into useVoice/useUpload (they mutate the draft and
   * then re-sync it).
   */
  syncDraft: () => void;

  /** settings.js `openSettings()` — reveals the settings overlay + reloads STT/version. */
  openSettings: () => void;

  /** settings.js `closeSettings()` — hides the settings overlay. */
  closeSettings: () => void;
}

// ---------------------------------------------------------------------------
// Context value
// ---------------------------------------------------------------------------

export interface AppStateContextValue {
  /** Mutable-by-reference `state` object (state.js `state`). */
  state: AppState;
  /** Mutable-by-reference `voice` object (state.js `voice`). */
  voice: VoiceState;
  /** Mutable-by-reference `upload` object (state.js `upload`). */
  upload: UploadState;
  /**
   * Re-render trigger. Call AFTER mutating any of the above where the original
   * called a render function. Bumps an internal version counter; every consumer
   * of `useAppState()` re-renders. Stable identity (safe in deps).
   */
  bump: () => void;
  /**
   * Monotonic render version. Components rarely read this directly — calling
   * `useAppState()` already subscribes to bumps — but it is exposed for
   * `useMemo`/`useEffect` deps that must recompute on every bump.
   */
  version: number;
  /** Shared callbacks implemented by App (see AppCallbacks). */
  callbacks: AppCallbacks;
}

const AppStateContext = createContext<AppStateContextValue | null>(null);

// noop callbacks — used only as the pre-wire default so a stray early consumer
// doesn't crash; App overwrites all of these via `setCallbacks` on mount.
const NOOP_CALLBACKS: AppCallbacks = {
  selectPane: () => {},
  wsSend: () => false,
  findPane: () => null,
  getSelectedPeer: () => "",
  showInputError: () => {},
  setInputStatus: () => {},
  setConnStatus: () => {},
  syncSelectionButton: () => {},
  syncDraft: () => {},
  openSettings: () => {},
  closeSettings: () => {},
};

/**
 * Provider. Owns the three mutable-by-reference objects (as refs), the version
 * counter, and a mutable callbacks slot that App fills in via the returned
 * `setCallbacks`. Render once at the App root; App immediately calls
 * `setCallbacks(...)` from a layout effect (or synchronously) with its wired
 * implementations.
 *
 * Usage in App.tsx:
 *   const store = useAppStateStore();      // returns { value, setCallbacks }
 *   <AppStateProvider store={store}> … </AppStateProvider>
 * Then App registers callbacks once the orchestration functions are defined.
 */
export function AppStateProvider({
  store,
  children,
}: {
  store: AppStateStore;
  children: ReactNode;
}) {
  return (
    <AppStateContext.Provider value={store.value}>
      {children}
    </AppStateContext.Provider>
  );
}

/**
 * App-side hook that creates the store: stable refs for state/voice/upload, a
 * version counter + bump, and a swappable callbacks slot. App calls this once,
 * passes `store` to AppStateProvider, and registers its callbacks via
 * `setCallbacks`. The returned `value` identity is stable except `version`,
 * which changes on every `bump()` so React re-renders subscribers.
 */
export interface AppStateStore {
  value: AppStateContextValue;
  /**
   * Replace the shared callbacks (App calls this once its orchestration
   * functions exist). Does NOT bump — callbacks are read fresh on each call via
   * the live `value.callbacks` reference.
   */
  setCallbacks: (cb: AppCallbacks) => void;
}

export function useAppStateStore(): AppStateStore {
  // Lazy-init refs (React 19 @types/react has no zero-arg useRef overload, so we
  // pass an explicit `undefined` and populate on first render). These objects
  // are created once and mutated in place forever after — never reassigned.
  const stateRef = useRef<AppState | undefined>(undefined);
  if (!stateRef.current) stateRef.current = initialState();
  const voiceRef = useRef<VoiceState | undefined>(undefined);
  if (!voiceRef.current) voiceRef.current = initialVoice();
  const uploadRef = useRef<UploadState | undefined>(undefined);
  if (!uploadRef.current) uploadRef.current = initialUpload();

  const callbacksRef = useRef<AppCallbacks>(NOOP_CALLBACKS);

  const [version, setVersion] = useState(0);
  const bump = useCallback(() => setVersion((v) => v + 1), []);

  const setCallbacks = useCallback((cb: AppCallbacks) => {
    callbacksRef.current = cb;
  }, []);

  // The value identity changes when `version` changes (so subscribers re-run);
  // the underlying state/voice/upload object references stay stable across the
  // component's lifetime (we mutate them in place). callbacks is read through
  // the ref so a setCallbacks after first render is observed without a bump.
  const value = useMemo<AppStateContextValue>(
    () => ({
      // Non-null assertions are safe: the refs are populated above before this
      // memo runs, and never cleared.
      state: stateRef.current!,
      voice: voiceRef.current!,
      upload: uploadRef.current!,
      bump,
      version,
      callbacks: callbacksRef.current,
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [version, bump],
  );

  return useMemo(() => ({ value, setCallbacks }), [value, setCallbacks]);
}

/**
 * Consumer hook. Every component/hook that reads or mutates app state calls
 * this. Returns the live context value; re-renders the calling component on
 * every `bump()`. Throws if used outside an AppStateProvider (programmer error).
 */
export function useAppState(): AppStateContextValue {
  const ctx = useContext(AppStateContext);
  if (!ctx) {
    throw new Error("useAppState must be used within an AppStateProvider");
  }
  return ctx;
}
