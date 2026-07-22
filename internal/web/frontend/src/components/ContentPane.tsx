// ContentPane — the React port of pre#content plus app.js's #content event
// wiring (selection mode, preview long-press, Cmd/Ctrl+click preview).
//
// IMPERATIVE HTML RULE (ARCHITECTURE.md §7): React must NEVER reconcile the
// pane output. The rendered HTML is produced by the PURE terminal renderer and
// assigned to pre#content.innerHTML in a useLayoutEffect, then markPreviewablePaths
// mutates the live DOM and the scroll position is restored — exactly as the
// original terminal.js setContent did:
//
//   const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 60;
//   pre.innerHTML = html;
//   markPreviewablePaths(pre, cwd, peer);
//   if (atBottom) pre.scrollTop = pre.scrollHeight;
//
// App owns paneLines/paneCache and pushes the latest {text, cwd, peer} down as
// props (see §7) — "setContent(...)" in the port = App updates these props,
// then this layout effect rewrites innerHTML.

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type {
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  UIEvent as ReactUIEvent,
} from "react";
import { render, markPreviewablePaths } from "../terminal/render";
import { preserveRenderedMermaidBlocks, renderMermaidDiagrams } from "../lib/mermaid";

// PREVIEW_LONG_PRESS_MS / _MOVE — verbatim from app.js image preview behavior.
const PREVIEW_LONG_PRESS_MS = 550;
const PREVIEW_LONG_PRESS_MOVE = 10;

// previewPress mirrors app.js's module-scoped `imagePress`: the in-flight
// long-press bookkeeping, mutated in place (refs, not state) so timing matches.
interface PreviewPress {
  target: HTMLElement;
  x: number;
  y: number;
  opened: boolean;
  timer: ReturnType<typeof setTimeout> | null;
}

interface PaneFrame {
  paneID: string | null;
  text: string;
  cwd: string | null;
  peer: string | null;
  markdown: boolean;
}

function sameFrame(a: PaneFrame | null, b: PaneFrame): boolean {
  return (
    !!a &&
    a.paneID === b.paneID &&
    a.text === b.text &&
    a.cwd === b.cwd &&
    a.peer === b.peer &&
    a.markdown === b.markdown
  );
}

export interface ContentPaneProps {
  /** Exact selected pane id; changes force an immediate, clean pane render. */
  paneID: string | null;
  /** Latest pane output text (App joins paneLines with "\n"). */
  text: string;
  /** Selected pane cwd (for preview path resolution); undefined/empty if none. */
  cwd?: string | null;
  /** Selected pane peer (for cross-host preview fetch); "" if local. */
  peer?: string | null;
  /** Markdown view: fold pipe tables into <table> + drop ANSI colours. */
  markdown: boolean;
  /** Mirrors state.selectionMode (App also owns the content-wrap class). */
  selectionMode: boolean;
  /**
   * app.js `previewImagePath(path, cwd, peer)` entry point. ImagePreview owner
   * (App) implements this; Cmd/Ctrl+click and long-press both route through it.
   */
  onPreviewImage: (path: string, cwd: string, peer: string) => void;
  /** App-owned markdown preview entry point. */
  onPreviewMarkdown: (path: string, cwd: string, peer: string) => void;
  /** App lazily reveals older buffered pane lines when the user scrolls upward. */
  onScrollTop: () => void;
  /**
   * Plain-click refocus path: app.js focused #direct-input then renderMode().
   * App implements (it owns #direct-input + the bump/renderMode).
   */
  onRefocusDirect: () => void;
  /**
   * Selection-mode path: app.js blurred #direct-input then renderMode().
   * App implements.
   */
  onBlurDirect: () => void;
}

export default function ContentPane({
  paneID,
  text,
  cwd,
  peer,
  markdown,
  selectionMode,
  onPreviewImage,
  onPreviewMarkdown,
  onScrollTop,
  onRefocusDirect,
  onBlurDirect,
}: ContentPaneProps) {
  const preRef = useRef<HTMLPreElement | null>(null);
  const previewPressRef = useRef<PreviewPress | null>(null);
  const pointerInteractionRef = useRef(false);
  const pointerUnlockTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const selectionModeRef = useRef(selectionMode);
  selectionModeRef.current = selectionMode;
  const activePaneRef = useRef<string | null>(paneID);
  const lastInputFrameRef = useRef<PaneFrame | null>(null);
  const committedFrameRef = useRef<PaneFrame | null>(null);
  const pendingFrameRef = useRef<PaneFrame | null>(null);
  const [frameDeferred, setFrameDeferred] = useState(false);
  // The first layout-effect run is the mount, BEFORE App has called setContent
  // even once. index.html shipped pre#content with a static placeholder
  // (`<span class="empty">No pane selected.</span>`) that app.js left in place
  // until the first setContent fired. The <pre> JSX below is intentionally
  // CHILDLESS (React must never reconcile pane output — ARCHITECTURE §3), so on
  // mount we write that placeholder IMPERATIVELY here, then every later run is a
  // real setContent (App only changes text/cwd/peer through setContent) and
  // overwrites exactly as the original did — including empty patches.
  const settledRef = useRef(false);

  const paneHasSelection = useCallback((): boolean => {
    const pre = preRef.current;
    const selection = window.getSelection();
    if (!pre || !selection || selection.isCollapsed || selection.rangeCount === 0) return false;
    return pre.contains(selection.anchorNode) || pre.contains(selection.focusNode);
  }, []);

  const clearPaneSelection = useCallback((): void => {
    if (!paneHasSelection()) return;
    window.getSelection()?.removeAllRanges();
  }, [paneHasSelection]);

  const commitFrame = useCallback((frame: PaneFrame): void => {
    const pre = preRef.current;
    if (!pre) return;
    const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 60;
    const html = render(frame.text, {
      cwd: frame.cwd || undefined,
      peer: frame.peer || undefined,
      markdown: frame.markdown,
    });
    pre.innerHTML = frame.markdown ? preserveRenderedMermaidBlocks(pre, html) : html;
    markPreviewablePaths(pre, frame.cwd, frame.peer);
    if (frame.markdown) void renderMermaidDiagrams(pre);
    if (atBottom) pre.scrollTop = pre.scrollHeight;
    committedFrameRef.current = frame;
  }, []);

  const renderLocked = useCallback((): boolean => {
    return pointerInteractionRef.current || selectionModeRef.current || paneHasSelection();
  }, [paneHasSelection]);

  const flushPendingFrame = useCallback((): void => {
    if (renderLocked()) return;
    const pending = pendingFrameRef.current;
    if (!pending) return;
    pendingFrameRef.current = null;
    commitFrame(pending);
    setFrameDeferred(false);
  }, [commitFrame, renderLocked]);

  const clearPointerUnlockTimer = useCallback((): void => {
    if (pointerUnlockTimerRef.current == null) return;
    clearTimeout(pointerUnlockTimerRef.current);
    pointerUnlockTimerRef.current = null;
  }, []);

  const clearPreviewPress = useCallback((): void => {
    const press = previewPressRef.current;
    if (press?.timer) clearTimeout(press.timer);
    previewPressRef.current = null;
  }, []);

  const unlockPointerInteraction = useCallback((): void => {
    clearPointerUnlockTimer();
    pointerInteractionRef.current = false;
    flushPendingFrame();
  }, [clearPointerUnlockTimer, flushPendingFrame]);

  const schedulePointerUnlock = useCallback((): void => {
    if (!pointerInteractionRef.current) return;
    clearPointerUnlockTimer();
    pointerUnlockTimerRef.current = setTimeout(() => {
      pointerUnlockTimerRef.current = null;
      pointerInteractionRef.current = false;
      flushPendingFrame();
    }, 0);
  }, [clearPointerUnlockTimer, flushPendingFrame]);

  // setContent — read atBottom BEFORE writing, assign innerHTML imperatively,
  // run markPreviewablePaths, restore scroll. Live frames keep arriving while
  // selection/pointer interaction is locked; only the newest one is retained.
  useLayoutEffect(() => {
    const pre = preRef.current;
    if (!pre) return;
    const frame: PaneFrame = {
      paneID,
      text,
      cwd: cwd || null,
      peer: peer || null,
      markdown,
    };
    if (!settledRef.current) {
      // Mount: no setContent yet — paint the index.html placeholder imperatively.
      settledRef.current = true;
      activePaneRef.current = paneID;
      lastInputFrameRef.current = frame;
      pre.innerHTML = '<span class="empty">No pane selected.</span>';
      return;
    }

    if (paneID !== activePaneRef.current) {
      // A pending frame belongs to the old pane. Clear every old interaction
      // before painting the selected pane immediately, even if a pointer or
      // selection-mode lock was active.
      clearPointerUnlockTimer();
      clearPreviewPress();
      pointerInteractionRef.current = false;
      clearPaneSelection();
      pendingFrameRef.current = null;
      activePaneRef.current = paneID;
      lastInputFrameRef.current = frame;
      commitFrame(frame);
      setFrameDeferred(false);
      return;
    }

    const frameChanged = !sameFrame(lastInputFrameRef.current, frame);
    lastInputFrameRef.current = frame;
    if (frameChanged) {
      if (sameFrame(committedFrameRef.current, frame)) {
        pendingFrameRef.current = null;
        setFrameDeferred(false);
        return;
      }
      if (renderLocked()) {
        pendingFrameRef.current = frame;
        setFrameDeferred(true);
        return;
      }
      pendingFrameRef.current = null;
      commitFrame(frame);
      setFrameDeferred(false);
      return;
    }

    // A lock prop may have changed without a new frame (selection mode off).
    flushPendingFrame();
  }, [
    paneID,
    text,
    cwd,
    peer,
    markdown,
    selectionMode,
    clearPaneSelection,
    clearPointerUnlockTimer,
    clearPreviewPress,
    commitFrame,
    flushPendingFrame,
    renderLocked,
  ]);

  useEffect(() => {
    const handleSelectionChange = (): void => flushPendingFrame();
    const handleDocumentPointerUp = (): void => schedulePointerUnlock();
    const handleDocumentPointerCancel = (): void => {
      if (!pointerInteractionRef.current) return;
      clearPreviewPress();
      unlockPointerInteraction();
    };
    document.addEventListener("selectionchange", handleSelectionChange);
    // Pointer release can land outside the pane after a drag. Capture it at the
    // document boundary so no interaction lock can remain orphaned.
    document.addEventListener("pointerup", handleDocumentPointerUp, true);
    document.addEventListener("pointercancel", handleDocumentPointerCancel, true);
    return () => {
      document.removeEventListener("selectionchange", handleSelectionChange);
      document.removeEventListener("pointerup", handleDocumentPointerUp, true);
      document.removeEventListener("pointercancel", handleDocumentPointerCancel, true);
      clearPointerUnlockTimer();
      clearPreviewPress();
    };
  }, [
    clearPointerUnlockTimer,
    clearPreviewPress,
    flushPendingFrame,
    schedulePointerUnlock,
    unlockPointerInteraction,
  ]);

  // ---- long-press / preview helpers (app.js image behavior, generalized) ----

  const previewTarget = (e: { target: EventTarget | null }): HTMLElement | null => {
    const t = e.target as (Element & { closest?: (s: string) => Element | null }) | null;
    return t && t.closest ? (t.closest(".image-path,.markdown-path") as HTMLElement | null) : null;
  };

  const openPreviewTarget = (target: HTMLElement): void => {
    const path = target.dataset.path || target.textContent || "";
    const targetCwd = target.dataset.cwd || "";
    const targetPeer = target.dataset.peer || "";
    if (target.dataset.kind === "markdown" || target.classList.contains("markdown-path")) {
      onPreviewMarkdown(path, targetCwd, targetPeer);
      return;
    }
    onPreviewImage(path, targetCwd, targetPeer);
  };

  // ---- #content event handlers (app.js wireInput content block) ----

  const handleMouseUp = (): void => {
    if (selectionMode) {
      onBlurDirect();
      return;
    }
    // After a shift-drag we want to leave focus alone — refocusing
    // #direct-input would clear the just-made selection in pre#content and the
    // user can't even copy it. The next plain click returns to direct mode.
    const sel = window.getSelection();
    const content = preRef.current;
    if (sel && !sel.isCollapsed && content && content.contains(sel.anchorNode)) return;
    onRefocusDirect();
  };

  const handleClick = (e: ReactMouseEvent): void => {
    try {
      const ip = previewPressRef.current;
      if (ip && ip.opened) {
        e.preventDefault();
        e.stopPropagation();
        clearPreviewPress();
        return;
      }
      const target = previewTarget(e);
      if (!target || !(e.metaKey || e.ctrlKey)) return;
      e.preventDefault();
      e.stopPropagation();
      openPreviewTarget(target);
    } finally {
      // Browser click dispatch follows pointerup. Keep the old target alive
      // through this handler, then release the repaint lock exactly once.
      unlockPointerInteraction();
    }
  };

  const handlePointerDown = (e: ReactPointerEvent): void => {
    clearPointerUnlockTimer();
    pointerInteractionRef.current = true;
    const target = previewTarget(e);
    if (!target || e.pointerType === "mouse") return;
    clearPreviewPress();
    const press: PreviewPress = {
      target,
      x: e.clientX,
      y: e.clientY,
      opened: false,
      timer: null,
    };
    press.timer = setTimeout(() => {
      const cur = previewPressRef.current;
      if (!cur || cur.target !== target) return;
      cur.opened = true;
      openPreviewTarget(target);
    }, PREVIEW_LONG_PRESS_MS);
    previewPressRef.current = press;
  };

  const handlePointerMove = (e: ReactPointerEvent): void => {
    const ip = previewPressRef.current;
    if (!ip) return;
    const dx = Math.abs(e.clientX - ip.x);
    const dy = Math.abs(e.clientY - ip.y);
    if (dx > PREVIEW_LONG_PRESS_MOVE || dy > PREVIEW_LONG_PRESS_MOVE) clearPreviewPress();
  };

  const handlePointerUp = (): void => {
    const ip = previewPressRef.current;
    if (ip && !ip.opened) clearPreviewPress();
    // `click` normally releases the lock after its target has dispatched. The
    // zero-delay fallback covers drag/no-click gestures without racing click.
    schedulePointerUnlock();
  };

  const handlePointerCancel = (): void => {
    clearPreviewPress();
    unlockPointerInteraction();
  };

  const handleScroll = (_e: ReactUIEvent<HTMLPreElement>): void => {
    onScrollTop();
  };

  // The pure renderer fills the inner HTML imperatively in the layout effect;
  // React must never receive it as children. The element starts empty here and
  // is populated synchronously before paint.
  return (
    <>
      <pre
        id="content"
        ref={preRef}
        onMouseUp={handleMouseUp}
        onClick={handleClick}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerCancel}
        onScroll={handleScroll}
      />
      {frameDeferred ? (
        <div className="pane-update-paused" role="status" aria-live="polite">
          Live updates paused while selecting
        </div>
      ) : null}
    </>
  );
}
