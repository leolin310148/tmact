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

import { useLayoutEffect, useRef } from "react";
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

export interface ContentPaneProps {
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
  // The first layout-effect run is the mount, BEFORE App has called setContent
  // even once. index.html shipped pre#content with a static placeholder
  // (`<span class="empty">No pane selected.</span>`) that app.js left in place
  // until the first setContent fired. The <pre> JSX below is intentionally
  // CHILDLESS (React must never reconcile pane output — ARCHITECTURE §3), so on
  // mount we write that placeholder IMPERATIVELY here, then every later run is a
  // real setContent (App only changes text/cwd/peer through setContent) and
  // overwrites exactly as the original did — including empty patches.
  const settledRef = useRef(false);

  // setContent — read atBottom BEFORE writing, assign innerHTML imperatively,
  // run markPreviewablePaths, restore scroll. Keyed on text/cwd/peer so it re-runs
  // exactly when app.js called setContent (a new paneLines join / pane switch).
  useLayoutEffect(() => {
    const pre = preRef.current;
    if (!pre) return;
    if (!settledRef.current) {
      // Mount: no setContent yet — paint the index.html placeholder imperatively.
      settledRef.current = true;
      pre.innerHTML = '<span class="empty">No pane selected.</span>';
      return;
    }
    const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 60;
    const html = render(text, { cwd: cwd || undefined, peer: peer || undefined, markdown });
    pre.innerHTML = markdown ? preserveRenderedMermaidBlocks(pre, html) : html;
    markPreviewablePaths(pre, cwd, peer);
    if (markdown) void renderMermaidDiagrams(pre);
    if (atBottom) pre.scrollTop = pre.scrollHeight;
  }, [text, cwd, peer, markdown]);

  // ---- long-press / preview helpers (app.js image behavior, generalized) ----

  const clearPreviewPress = (): void => {
    const ip = previewPressRef.current;
    if (ip && ip.timer) clearTimeout(ip.timer);
    previewPressRef.current = null;
  };

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
  };

  const handlePointerDown = (e: ReactPointerEvent): void => {
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
    if (!ip || ip.opened) return;
    clearPreviewPress();
  };

  const handlePointerCancel = (): void => {
    clearPreviewPress();
  };

  const handleScroll = (_e: ReactUIEvent<HTMLPreElement>): void => {
    onScrollTop();
  };

  // The pure renderer fills the inner HTML imperatively in the layout effect;
  // React must never receive it as children. The element starts empty here and
  // is populated synchronously before paint.
  return (
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
  );
}
