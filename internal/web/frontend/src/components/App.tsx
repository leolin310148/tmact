// App — the orchestrator. 1:1 behavioral port of static/app.js.
//
// This is the linchpin of the React migration: it owns the AppState store,
// implements EVERY shared callback (ARCHITECTURE.md §4), owns paneLines /
// paneCache / openWS / closeWS / applySnapshot, wires every feature hook with
// its injected-deps object (§3), and composes the full DOM tree in the exact
// order of static/index.html (§9). The original re-ran imperative render
// functions after a mutation; here those become React renders driven by the
// store's bump() (§10) plus a handful of App-local render values held in refs
// and surfaced via a local `tick` bump (connStatus / mode strings / option-bar
// question / the ContentPane text+opts).
//
// PARITY MODEL (read ARCHITECTURE.md §0/§7/§9/§10 first):
//   - state/voice/upload are mutated BY REFERENCE (store refs) exactly like
//     state.js. Re-render via bump().
//   - Module-scoped mutable values from app.js live in refs:
//       paneLines, paneCache, errorTimer, ctrlArmed (shared with KeyBar/sendDirect),
//       imagePreview src/path, the App-local render values (conn/mode/question/pane).
//   - #content-wrap classes (.direct / .selection-mode) are toggled IMPERATIVELY
//     in a layout effect (app.js renderMode used classList.toggle), so they never
//     clobber the `.upload-ready` class that useQuick.syncQuickDock toggles on the
//     same element (also imperative). #content-wrap is therefore an uncontrolled
//     element (we set only its base className once + toggle the three flags
//     imperatively), exactly like the original.
//   - #draft / #direct-input / #record-btn / #send-btn are uncontrolled DOM nodes
//     that App mutates by ref (selectPane sets .value/.disabled, autoGrowDraft
//     sizes #draft, syncRecordButton mutates #record-btn). React never controls
//     their value/disabled state — identical to app.js.

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type {
  ClipboardEvent as ReactClipboardEvent,
  CompositionEvent as ReactCompositionEvent,
  FormEvent as ReactFormEvent,
  KeyboardEvent as ReactKeyboardEvent,
} from "react";

import {
  AppStateProvider,
  useAppStateStore,
  useAppState,
} from "../store/AppStateContext";
import type { AppCallbacks } from "../store/AppStateContext";
import type { InputMsg, PaneStatus, Question, Snapshot } from "../types/server";
import { isMobile } from "../lib/dom";
import { translateKey } from "../lib/keymap";

import { StatusLine, panePeer } from "./StatusLine";
import { ConnStatus } from "./ConnStatus";
import { StaleDot } from "./StaleDot";
import { OptionBar } from "./OptionBar";
import ContentPane from "./ContentPane";
import CopyLineBar from "./CopyLineBar";
import ImagePreview, { buildImageSrc } from "./ImagePreview";
import InputBar from "./InputBar";
import Draft from "./Draft";
import DirectInput from "./DirectInput";
import ModeIndicator from "./ModeIndicator";
import KeyBar from "./KeyBar";
import { QuickDock } from "./QuickDock";
import { QuickEditor } from "./QuickEditor";
import { RecOverlay } from "./RecOverlay";
import UsagePanel from "./UsagePanel";
import SettingsDialog from "./SettingsDialog";
import { HelpOverlay } from "./HelpOverlay";
import { UploadControls } from "./UploadControls";

import { usePaneStream } from "../ws/usePaneStream";
import type { ConnState } from "../ws/usePaneStream";
import { useSnapshotStream } from "../ws/useSnapshotStream";
import { useVoice } from "../hooks/useVoice";
import { useUpload } from "../hooks/useUpload";
import { useQuick } from "../hooks/useQuick";
import { useSettings } from "../hooks/useSettings";
import { useHelp } from "../hooks/useHelp";
import { useHotkeys } from "../hooks/useHotkeys";
import { useViewport } from "../hooks/useViewport";

// Persisted-selection localStorage key — verbatim from app.js (SELECTED_KEY).
const SELECTED_KEY = "tmact.selectedPane";

// findPane scans the live snapshot for a pane by id — verbatim from app.js.
function findPaneIn(snap: Snapshot | null, paneID: string | null): PaneStatus | null {
  if (!snap || !paneID) return null;
  for (const t in snap.panes) {
    const p = snap.panes[t];
    if (p && p.pane_id === paneID) return p;
  }
  return null;
}

// AppInner runs inside the provider so it can call useAppState() (which the
// feature hooks also call). App (below) creates the store and wraps this.
function AppInner({ store }: { store: ReturnType<typeof useAppStateStore> }) {
  const { state, bump } = useAppState();

  // ----- App-local render values (app.js wrote these to the DOM; here they are
  // refs + a local `tick` bump that re-renders the presentational shells). -----
  const [, setTick] = useState(0);
  const renderLocal = useCallback(() => setTick((n) => n + 1), []);

  // conn-status strip text (setConnStatus). mode strings (renderMode/showInputError
  // /setInputStatus). option-bar question (renderOptions). All read during render.
  const connStatusRef = useRef("");
  const modeTextRef = useRef("");
  const inputErrorRef = useRef("");
  const questionRef = useRef<Question | null>(null);

  // ContentPane text+opts (the "setContent" surface — §7). Held in a ref + bump so
  // ContentPane's layout effect rewrites pre#content.innerHTML imperatively.
  const paneContentRef = useRef<{ text: string; cwd: string | null; peer: string | null }>({
    text: "",
    cwd: null,
    peer: null,
  });

  // image-preview lightbox (app.js previewImagePath/close).
  const [imageSrc, setImageSrc] = useState<string | null>(null);
  const imagePathRef = useRef<string>("");

  // ----- module-scoped mutable state from app.js → refs -----
  const paneLinesRef = useRef<string[]>([]);
  const paneCacheRef = useRef<Record<string, string[]>>({});
  const errorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const ctrlArmedRef = useRef(false);

  // Uncontrolled DOM node refs (App mutates .value/.disabled imperatively).
  const contentWrapRef = useRef<HTMLDivElement | null>(null);
  const draftRef = useRef<HTMLTextAreaElement | null>(null);
  const draftWrapRef = useRef<HTMLDivElement | null>(null);
  const directRef = useRef<HTMLTextAreaElement | null>(null);
  const recordBtnRef = useRef<HTMLButtonElement | null>(null);
  const sendBtnRef = useRef<HTMLButtonElement | null>(null);

  // ----- setContent (§7) -----
  // app.js setContent(text, opts) replaced pre#content.innerHTML; here we push
  // {text,cwd,peer} into the ref and re-render so ContentPane's layout effect
  // rewrites the inner HTML. The two-arg form (setContent("Loading…")) drops the
  // cwd/peer (null), matching app.js where opts is undefined.
  const setContent = useCallback(
    (text: string, opts?: { cwd?: string | null; peer?: string | null }) => {
      paneContentRef.current = {
        text,
        cwd: opts && opts.cwd != null ? opts.cwd : null,
        peer: opts && opts.peer != null ? opts.peer : null,
      };
      renderLocal();
    },
    [renderLocal],
  );

  // ----- option bar (renderOptions) -----
  const renderOptions = useCallback(
    (q: Question | null) => {
      questionRef.current = q && Array.isArray(q.choices) ? q : null;
      renderLocal();
    },
    [renderLocal],
  );

  // ----- error / status strips (§4, §6 item 83) -----
  const syncIndicator = useCallback(() => {
    // In the port the ModeIndicator computes its own visibility from the two
    // strings on render; this just re-renders it (the original toggled display).
    renderLocal();
  }, [renderLocal]);

  const showInputError = useCallback(
    (msg: string) => {
      inputErrorRef.current = msg;
      syncIndicator();
      if (errorTimerRef.current) clearTimeout(errorTimerRef.current);
      errorTimerRef.current = setTimeout(() => {
        inputErrorRef.current = "";
        syncIndicator();
      }, 6000);
    },
    [syncIndicator],
  );

  const setInputStatus = useCallback(
    (msg: string) => {
      if (errorTimerRef.current) {
        clearTimeout(errorTimerRef.current);
        errorTimerRef.current = null;
      }
      inputErrorRef.current = msg;
      syncIndicator();
    },
    [syncIndicator],
  );

  const setConnStatus = useCallback(
    (msg: string) => {
      connStatusRef.current = msg;
      renderLocal();
    },
    [renderLocal],
  );

  // ----- pane lookup helpers (§4) -----
  const findPane = useCallback(
    (paneID: string | null): PaneStatus | null => findPaneIn(state.snapshot, paneID),
    [state],
  );
  const getSelectedPeer = useCallback(
    (): string => panePeer(findPaneIn(state.snapshot, state.selected)),
    [state],
  );

  // ----- pane WS stream (§3) -----
  // wsSend = paneStream.send. The stream is created with App's callbacks; onPatch
  // splices paneLines, caches, drives setContent + renderOptions (app.js block).
  const paneStream = usePaneStream({
    getSelectedPane: () => state.selected,
    onPatch: (from, lines, question) => {
      paneLinesRef.current = paneLinesRef.current.slice(0, from).concat(lines);
      if (state.selected) paneCacheRef.current[state.selected] = paneLinesRef.current;
      const p = findPaneIn(state.snapshot, state.selected);
      setContent(paneLinesRef.current.join("\n"), { cwd: p && p.cwd, peer: panePeer(p) });
      renderOptions(question);
    },
    onQuestion: renderOptions,
    onError: showInputError,
    onStatus: (s: ConnState) => {
      // Surface the connection state in the strip above the chips; the input-bar
      // error/upload slot is independent and stays put (app.js onStatus block).
      if (s === "connecting") {
        setConnStatus("connecting…");
        if (paneLinesRef.current.length === 0) setContent("Connecting…");
      } else if (s === "reconnecting") {
        setConnStatus("reconnecting…");
        if (paneLinesRef.current.length === 0) setContent("Reconnecting…");
      } else if (s === "open") {
        setConnStatus("");
      }
    },
  });

  const wsSend = useCallback(
    (obj: InputMsg): boolean => paneStream.send(obj),
    [paneStream],
  );

  // ----- openWS / closeWS (§7, App-local) -----
  const closeWS = useCallback(() => {
    paneStream.close();
  }, [paneStream]);

  const openWS = useCallback(
    (paneID: string) => {
      // Seed from the cache so a revisited pane shows content immediately; the
      // first patch (from=0) replaces it. A never-seen pane stays empty.
      const cached = paneCacheRef.current[paneID];
      paneLinesRef.current = cached ? cached.slice() : [];
      if (paneLinesRef.current.length) {
        const p = findPaneIn(state.snapshot, paneID);
        setContent(paneLinesRef.current.join("\n"), { cwd: p && p.cwd, peer: panePeer(p) });
      }
      paneStream.open(paneID);
    },
    [paneStream, state, setContent],
  );

  // ----- syncDraft / autoGrowDraft (§4, imperative — synchronous scrollHeight) ----
  const autoGrowDraft = useCallback(() => {
    const draft = draftRef.current;
    if (!draft) return;
    draft.style.height = "auto";
    const cs = getComputedStyle(draft);
    const max = parseFloat(cs.maxHeight) || 200;
    const border = parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth);
    const full = draft.scrollHeight + border;
    draft.style.height = Math.min(full, max) + "px";
    draft.style.overflowY = full > max ? "auto" : "hidden";
  }, []);

  const syncDraft = useCallback(() => {
    const draft = draftRef.current;
    const wrap = draftWrapRef.current;
    if (wrap && draft) {
      wrap.classList.toggle("has-text", !draft.disabled && draft.value !== "");
    }
    autoGrowDraft();
  }, [autoGrowDraft]);

  // ----- selection button (§4) -----
  const syncSelectionButton = useCallback(() => {
    const btn = document.getElementById("selection-btn") as HTMLButtonElement | null;
    if (!btn) return;
    btn.classList.toggle("ready", !!state.selected);
    btn.classList.toggle("active", state.selectionMode);
    btn.disabled = !state.selected;
    btn.setAttribute("aria-pressed", state.selectionMode ? "true" : "false");
    btn.title = state.selectionMode ? "selection mode on" : "selection mode";
  }, [state]);

  // ----- settings (§3, drives openSettings/closeSettings callbacks) -----
  const settings = useSettings();
  const settingsRef = useRef(settings);
  settingsRef.current = settings;
  const openSettings = useCallback(() => settingsRef.current.openSettings(), []);
  const closeSettings = useCallback(() => settingsRef.current.closeSettings(), []);

  // ----- voice (§3) -----
  const voice = useVoice({ showInputError, syncDraft });
  const {
    syncRecordButton,
    positionRecOverlay,
    startRecording,
    stopRecording,
    cancelRecording,
    finishRecordingConfirm,
    wireRecordHotkey,
  } = voice;

  // ----- quick (§3) -----
  const quick = useQuick({ wsSend, showInputError, findPane, syncSelectionButton });
  const { syncQuickDock, closeQuickMenu, loadQuickConfig, wireQuick } = quick;

  // ----- upload (§3) -----
  const upload = useUpload({
    setInputStatus,
    showInputError,
    syncDraft,
    wsSend,
    getSelectedPeer,
  });
  const {
    clipboardImage,
    pasteImage,
    uploadFilesToPane,
    openFileUploadPicker,
    placeInDraft,
  } = upload;

  // ----- renderMode (§10) -----
  // app.js renderMode set the draft placeholder imperatively, computed `direct`,
  // toggled .direct on #input-bar / #mode-indicator / #content-wrap and
  // .selection-mode on #content-wrap, set #mode-text, then syncIndicator(). In the
  // port the .direct/.selection-mode classes on #input-bar / #mode-indicator are
  // props (computed below in render); #content-wrap is toggled imperatively (so it
  // doesn't clobber useQuick's .upload-ready). The placeholder + mode-text + a
  // re-render happen here.
  const renderMode = useCallback(() => {
    const mobile = isMobile();
    const draft = draftRef.current;
    if (draft) {
      draft.placeholder = mobile
        ? "Type a prompt, then tap Send"
        : "Type a prompt — ⌘/Ctrl+Enter to send";
    }
    modeTextRef.current = state.selected ? "" : "Select a pane to enable input";
    renderLocal(); // re-renders InputBar/ModeIndicator (read `direct`) + content-wrap effect
  }, [state, renderLocal]);

  // Computed during render so the presentational shells stay in step with focus
  // and selection state (app.js read document.activeElement live in renderMode).
  const directMode =
    !!state.selected &&
    !state.selectionMode &&
    typeof document !== "undefined" &&
    document.activeElement === directRef.current;

  // #content-wrap class toggles — imperative, so .upload-ready (owned by
  // syncQuickDock) is never clobbered. Mirrors renderMode's wrap.classList.toggle.
  useLayoutEffect(() => {
    const wrap = contentWrapRef.current;
    if (!wrap) return;
    wrap.classList.toggle("direct", directMode);
    wrap.classList.toggle("selection-mode", state.selectionMode);
  });

  // ----- selection persistence (§5) -----
  const rememberSelection = useCallback(
    (paneID: string) => {
      const p = findPaneIn(state.snapshot, paneID);
      try {
        localStorage.setItem(
          SELECTED_KEY,
          JSON.stringify({ pane: paneID, session: p ? p.session : "" }),
        );
      } catch (e) {
        /* ignore — quota / private mode (verbatim) */
      }
    },
    [state],
  );

  // ----- selectPane (§4) -----
  const selectPane = useCallback(
    (paneID: string) => {
      if (!paneID) return;
      if (paneID === state.selected) {
        // Re-selecting forces a reconnect; keep the cached output on screen.
        openWS(paneID);
        return;
      }
      state.selected = paneID;
      rememberSelection(paneID);
      const draft = draftRef.current;
      if (draft) {
        draft.value = state.drafts[paneID] || "";
        draft.disabled = false;
      }
      if (sendBtnRef.current) sendBtnRef.current.disabled = false;
      const uploadBtn = document.getElementById("upload-btn") as HTMLButtonElement | null;
      if (uploadBtn) uploadBtn.disabled = false;
      syncDraft();
      syncRecordButton();
      syncSelectionButton();
      setContent("Loading…");
      bump(); // renderStatusline(state.snapshot)
      renderMode();
      closeQuickMenu();
      syncQuickDock();
      // The selected chip scrolls itself into view via Chip's layout effect.
      openWS(paneID);
      // Desktop non-selection-mode: drop straight into direct mode.
      if (!isMobile() && !state.selectionMode && directRef.current) directRef.current.focus();
    },
    [
      state,
      openWS,
      rememberSelection,
      syncDraft,
      syncRecordButton,
      syncSelectionButton,
      setContent,
      bump,
      renderMode,
      closeQuickMenu,
      syncQuickDock,
    ],
  );

  // ----- draft / send / clear (App-local) -----
  const clearDraft = useCallback(() => {
    const draft = draftRef.current;
    if (!draft) return;
    draft.value = "";
    if (state.selected) delete state.drafts[state.selected];
    syncDraft();
    draft.focus();
  }, [state, syncDraft]);

  const sendDraft = useCallback(() => {
    if (!state.selected) return;
    const draft = draftRef.current;
    if (!draft) return;
    if (!draft.value.trim()) return;
    if (!wsSend({ t: "send", s: draft.value })) {
      showInputError("not connected — try again");
      return;
    }
    draft.value = "";
    delete state.drafts[state.selected];
    syncDraft();
  }, [state, wsSend, showInputError, syncDraft]);

  const clearPaneOutput = useCallback(() => {
    if (!state.selected) return;
    if (!wsSend({ t: "clear" })) showInputError("not connected — try again");
  }, [state, wsSend, showInputError]);

  const toggleSelectionMode = useCallback(() => {
    if (!state.selected) return;
    state.selectionMode = !state.selectionMode;
    if (state.selectionMode && directRef.current) directRef.current.blur();
    syncSelectionButton();
    renderMode();
  }, [state, syncSelectionButton, renderMode]);

  // ----- direct-mode relay (sendDirect, ctrl folding) -----
  const setCtrl = useCallback(
    (on: boolean) => {
      ctrlArmedRef.current = on;
      renderLocal(); // KeyBar reads ctrlArmedRef on render → .armed class
    },
    [renderLocal],
  );

  const sendDirect = useCallback(
    (msg: InputMsg) => {
      let out: InputMsg = msg;
      if (ctrlArmedRef.current && msg.t === "text" && msg.s.length === 1) {
        const c = msg.s.toLowerCase();
        if (c >= "a" && c <= "z") out = { t: "key", k: "C-" + c };
      }
      if (!wsSend(out)) showInputError("not connected — try again");
      if (ctrlArmedRef.current) setCtrl(false);
    },
    [wsSend, showInputError, setCtrl],
  );

  // ----- image preview (app.js previewImagePath/openImageTarget) -----
  const previewImagePath = useCallback((path: string, cwd: string, peer: string) => {
    imagePathRef.current = path;
    setImageSrc(buildImageSrc(path, cwd, peer));
  }, []);
  const closeImagePreview = useCallback(() => {
    setImageSrc(null);
  }, []);

  // ----- ContentPane focus handlers (mouseup refocus / blur) -----
  const onRefocusDirect = useCallback(() => {
    if (directRef.current) directRef.current.focus();
    // mouseup's plain-click path focuses #direct-input; renderMode follows via
    // the focusin listener (app.js relied on the same focus → renderMode chain).
  }, []);
  const onBlurDirect = useCallback(() => {
    if (directRef.current) directRef.current.blur();
    renderMode();
  }, [renderMode]);

  // ----- Draft event handlers (app.js wireInput draft block) -----
  const onDraftInput = useCallback(() => {
    const draft = draftRef.current;
    if (!draft) return;
    if (state.selected) state.drafts[state.selected] = draft.value;
    syncDraft();
  }, [state, syncDraft]);

  const onDraftKeyDown = useCallback(
    (e: ReactKeyboardEvent<HTMLTextAreaElement>) => {
      const draft = draftRef.current;
      if (!draft) return;
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        sendDraft();
        return;
      }
      if (
        !e.nativeEvent.isComposing &&
        e.key === "Enter" &&
        !e.shiftKey &&
        state.selected &&
        !draft.value.trim()
      ) {
        e.preventDefault();
        state.selectionMode = false;
        syncSelectionButton();
        if (directRef.current) directRef.current.focus();
        renderMode();
        sendDirect({ t: "key", k: "Enter" });
      }
    },
    [state, sendDraft, syncSelectionButton, renderMode, sendDirect],
  );

  const onDraftPaste = useCallback(
    (e: ReactClipboardEvent<HTMLTextAreaElement>) => {
      const img = clipboardImage(e.nativeEvent);
      if (!img) return;
      e.preventDefault();
      void pasteImage(img, placeInDraft);
    },
    [clipboardImage, pasteImage, placeInDraft],
  );

  // ----- DirectInput event handlers (app.js wireInput direct block) -----
  const onDirectKeyDown = useCallback(
    (e: ReactKeyboardEvent<HTMLTextAreaElement>) => {
      const direct = directRef.current;
      if (!direct) return;
      if (state.selectionMode) {
        direct.blur();
        renderMode();
        return;
      }
      if (e.nativeEvent.isComposing || e.keyCode === 229) return; // let the IME compose
      const msg = translateKey(e.nativeEvent);
      if (!msg) return;
      e.preventDefault();
      sendDirect(msg);
    },
    [state, renderMode, sendDirect],
  );

  const onDirectComposition = useCallback(
    (e: ReactCompositionEvent<HTMLTextAreaElement>) => {
      if (e.data) sendDirect({ t: "text", s: e.data });
      if (directRef.current) directRef.current.value = "";
    },
    [sendDirect],
  );

  const onDirectPaste = useCallback(
    (e: ReactClipboardEvent<HTMLTextAreaElement>) => {
      e.preventDefault();
      const img = clipboardImage(e.nativeEvent);
      if (img) {
        // Send the saved path plus a trailing space so the agent's input box
        // keeps it as one token, separate from whatever is typed next.
        void pasteImage(img, (path: string) => sendDirect({ t: "text", s: path + " " }));
        return;
      }
      const t = e.clipboardData.getData("text");
      if (t) sendDirect({ t: "text", s: t });
    },
    [clipboardImage, pasteImage, sendDirect],
  );

  const onDirectInput = useCallback(
    (e: ReactFormEvent<HTMLTextAreaElement>) => {
      if (e.nativeEvent && (e.nativeEvent as InputEvent).isComposing) return;
      const direct = directRef.current;
      if (!direct) return;
      const v = direct.value;
      direct.value = "";
      if (v) sendDirect({ t: "text", s: v });
    },
    [sendDirect],
  );

  // ----- file input change -----
  const onFiles = useCallback(
    (files: File[]) => {
      void uploadFilesToPane(files);
    },
    [uploadFilesToPane],
  );

  // ----- snapshot stream (§3) -----
  const snapshot = useSnapshotStream({
    paneCache: paneCacheRef,
    selectPane,
    syncQuickDock,
    renderMode,
    closeWS,
    openWS,
  });
  const { refreshSnapshot, startSnapshotStream } = snapshot;

  // ----- help (§3) -----
  const help = useHelp();

  // ----- hotkeys (§3, capture-phase — see §9 step 5) -----
  const settingsOpen = useCallback(() => settingsRef.current.visible, []);
  useHotkeys({ selectPane, clearPaneOutput, settingsOpen });

  // ----- viewport (§3) -----
  useViewport({ positionRecOverlay });

  // ===== register the shared callbacks (§4) — once, synchronously =====
  // setCallbacks is read through a ref in the store, so a post-first-render
  // registration is observed without a bump. We keep the latest implementations
  // current on every render (callback identities can change), exactly mirroring
  // app.js where the functions closed over the live module scope.
  const callbacks: AppCallbacks = useMemo(
    () => ({
      selectPane,
      wsSend,
      findPane,
      getSelectedPeer,
      showInputError,
      setInputStatus,
      setConnStatus,
      syncSelectionButton,
      syncDraft,
      openSettings,
      closeSettings,
    }),
    [
      selectPane,
      wsSend,
      findPane,
      getSelectedPeer,
      showInputError,
      setInputStatus,
      setConnStatus,
      syncSelectionButton,
      syncDraft,
      openSettings,
      closeSettings,
    ],
  );
  store.setCallbacks(callbacks);

  // ===== wiring order (§9) =====
  // Step 1 — synchronous, before first content paint: loadClientSettings +
  // loadQuickConfig. A layout effect that runs before the first snapshot render
  // applies --pane-font / data-running-effect and seeds the quick config (the
  // original called both first, synchronously). Runs once.
  const bootRef = useRef(false);
  useLayoutEffect(() => {
    if (bootRef.current) return;
    bootRef.current = true;
    settingsRef.current.loadClientSettings();
    loadQuickConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Steps 4–6 — wire input-side listeners that the original attached in
  // wireInput / wireCopyLine / wireQuick (these own document-level listeners),
  // wireRecordHotkey (capture-phase, before #direct-input's bubble keydown), and
  // the initial syncRecordButton/syncDraft. Run once on mount.
  //
  // ORDER: wireRecordHotkey + useHotkeys both register capture-phase keydown
  // BEFORE #direct-input's bubble-phase keydown (which React attaches when
  // DirectInput mounts). useHotkeys is a hook that adds its capture listener in
  // its own effect (mounted above); wireRecordHotkey adds its capture listeners
  // here. Both fire before the bubble-phase relay regardless of registration
  // order — capture always precedes bubble — matching app.js (§9 step 5).
  const wiredRef = useRef(false);
  useEffect(() => {
    if (wiredRef.current) return;
    wiredRef.current = true;
    wireQuick();
    wireRecordHotkey();
    syncRecordButton();
    syncDraft();
    // #send-btn carries no static `disabled` prop (a static one would make React
    // suppress its onClick forever — see InputBar's PARITY MODEL). selectPane is
    // the only path that enables it, so seed the initial DOM-disabled state here
    // to match the original "no pane selected → send disabled" markup.
    if (sendBtnRef.current) sendBtnRef.current.disabled = !state.selected;

    // app.js wired document focusin → renderMode and focusout → setTimeout(renderMode,0)
    // so the .direct class follows focus into/out of #direct-input.
    const onFocusIn = () => renderMode();
    const onFocusOut = () => setTimeout(() => renderMode(), 0);
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);

    // window resize: app.js re-ran positionRecOverlay + autoGrowDraft.
    const onResize = () => {
      positionRecOverlay();
      autoGrowDraft();
    };
    window.addEventListener("resize", onResize);

    return () => {
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("focusout", onFocusOut);
      window.removeEventListener("resize", onResize);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Step 8 — start snapshot delivery: refresh once immediately, then start SSE
  // if the tab is visible. Run once on mount, after the boot layout effect has
  // applied client settings (effects run after layout effects). Step 9 visibility
  // lifecycle lives inside useSnapshotStream (which also owns the WS reopen via
  // the injected openWS/closeWS).
  const startedRef = useRef(false);
  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    void refreshSnapshot();
    if (!document.hidden) startSnapshotStream();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ===== render the full DOM tree (index.html order) =====
  const pc = paneContentRef.current;

  return (
    <>
      {/* #content-wrap — uncontrolled className base; .direct/.selection-mode/
          .upload-ready toggled imperatively (layout effect + useQuick). */}
      <div className="content-wrap" id="content-wrap" ref={contentWrapRef}>
        <UsagePanel />
        <ContentPane
          text={pc.text}
          cwd={pc.cwd}
          peer={pc.peer}
          selectionMode={state.selectionMode}
          onPreviewImage={previewImagePath}
          onRefocusDirect={onRefocusDirect}
          onBlurDirect={onBlurDirect}
        />
        <DirectInput
          directRef={directRef}
          onDirectKeyDown={onDirectKeyDown}
          onDirectComposition={onDirectComposition}
          onDirectPaste={onDirectPaste}
          onDirectInput={onDirectInput}
        />
        {/* #help-btn lives inside #content-wrap (index.html). UploadControls
            emits it; HelpOverlay renders only the overlay (renderButton={false}). */}
        <UploadControls
          onUpload={openFileUploadPicker}
          onSelection={toggleSelectionMode}
          onClear={clearPaneOutput}
          onHelp={help.toggle}
          onFiles={onFiles}
        />
        <QuickDock quick={quick} />
        <CopyLineBar />
      </div>

      <ConnStatus text={connStatusRef.current} />
      <OptionBar
        question={questionRef.current}
        onChoose={(n) => {
          if (!wsSend({ t: "text", s: String(n) })) {
            showInputError("not connected — try again");
          }
        }}
      />

      <nav className="statusline">
        <StatusLine />
        <StaleDot />
        <button
          className="gear-btn"
          id="gear-btn"
          type="button"
          title="settings"
          aria-label="settings"
          onClick={openSettings}
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </nav>

      <InputBar
        keyBar={
          <KeyBar
            wsSend={wsSend}
            showInputError={showInputError}
            ctrlArmedRef={ctrlArmedRef}
            bump={renderLocal}
          />
        }
        modeIndicator={
          <ModeIndicator
            modeText={modeTextRef.current}
            inputError={inputErrorRef.current}
            direct={directMode}
          />
        }
        draft={
          <Draft
            draftRef={draftRef}
            draftWrapRef={draftWrapRef}
            onDraftInput={onDraftInput}
            onDraftKeyDown={onDraftKeyDown}
            onDraftPaste={onDraftPaste}
            onClearDraft={clearDraft}
          />
        }
        direct={directMode}
        recordBtnRef={recordBtnRef}
        sendBtnRef={sendBtnRef}
        onRecord={() => void startRecording({ confirmOnStop: false })}
        onSend={sendDraft}
      />

      <RecOverlay
        onStop={stopRecording}
        onSend={() => finishRecordingConfirm(true)}
        onCancel={cancelRecording}
      />

      <HelpOverlay help={help} renderButton={false} />

      <SettingsDialog settings={settings} quickEditor={<QuickEditor quick={quick} />} />

      <ImagePreview src={imageSrc} path={imagePathRef.current} onClose={closeImagePreview} />
    </>
  );
}

export default function App() {
  const store = useAppStateStore();
  return (
    <AppStateProvider store={store}>
      <AppInner store={store} />
    </AppStateProvider>
  );
}
