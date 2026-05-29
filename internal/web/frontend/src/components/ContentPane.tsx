// ContentPane — the React port of pre#content plus app.js's #content event
// wiring (selection mode, image long-press, Cmd+click preview).
//
// IMPERATIVE HTML RULE (ARCHITECTURE.md §7): React must NEVER reconcile the
// pane output. The rendered HTML is produced by the PURE terminal renderer and
// assigned to pre#content.innerHTML in a useLayoutEffect, then markImagePaths
// mutates the live DOM and the scroll position is restored — exactly as the
// original terminal.js setContent did:
//
//   const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 60;
//   pre.innerHTML = html;
//   markImagePaths(pre, cwd, peer);
//   if (atBottom) pre.scrollTop = pre.scrollHeight;
//
// App owns paneLines/paneCache and pushes the latest {text, cwd, peer} down as
// props (see §7) — "setContent(...)" in the port = App updates these props,
// then this layout effect rewrites innerHTML.

import { useLayoutEffect, useRef } from "react";
import type { MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from "react";
import { render, markImagePaths } from "../terminal/render";

// IMAGE_LONG_PRESS_MS / _MOVE — verbatim from app.js.
const IMAGE_LONG_PRESS_MS = 550;
const IMAGE_LONG_PRESS_MOVE = 10;

// imagePress mirrors app.js's module-scoped `imagePress`: the in-flight
// long-press bookkeeping, mutated in place (refs, not state) so timing matches.
interface ImagePress {
  target: HTMLElement;
  x: number;
  y: number;
  opened: boolean;
  timer: ReturnType<typeof setTimeout> | null;
}

export interface ContentPaneProps {
  /** Latest pane output text (App joins paneLines with "\n"). */
  text: string;
  /** Selected pane cwd (for image-path resolution); undefined/empty if none. */
  cwd?: string | null;
  /** Selected pane peer (for cross-host image fetch); "" if local. */
  peer?: string | null;
  /** Mirrors state.selectionMode (App also owns the content-wrap class). */
  selectionMode: boolean;
  /**
   * app.js `previewImagePath(path, cwd, peer)` entry point. ImagePreview owner
   * (App) implements this; Cmd+click and long-press both route through it.
   */
  onPreviewImage: (path: string, cwd: string, peer: string) => void;
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
  selectionMode,
  onPreviewImage,
  onRefocusDirect,
  onBlurDirect,
}: ContentPaneProps) {
  const preRef = useRef<HTMLPreElement | null>(null);
  const imagePressRef = useRef<ImagePress | null>(null);
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
  // run markImagePaths, restore scroll. Keyed on text/cwd/peer so it re-runs
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
    pre.innerHTML = render(text, { cwd: cwd || undefined, peer: peer || undefined });
    markImagePaths(pre, cwd, peer);
    if (atBottom) pre.scrollTop = pre.scrollHeight;
  }, [text, cwd, peer]);

  // ---- image long-press / preview helpers (app.js) ----

  const clearImagePress = (): void => {
    const ip = imagePressRef.current;
    if (ip && ip.timer) clearTimeout(ip.timer);
    imagePressRef.current = null;
  };

  const imageTarget = (e: { target: EventTarget | null }): HTMLElement | null => {
    const t = e.target as (Element & { closest?: (s: string) => Element | null }) | null;
    return t && t.closest ? (t.closest(".image-path") as HTMLElement | null) : null;
  };

  const openImageTarget = (target: HTMLElement): void => {
    onPreviewImage(
      target.dataset.path || target.textContent || "",
      target.dataset.cwd || "",
      target.dataset.peer || "",
    );
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
    const ip = imagePressRef.current;
    if (ip && ip.opened) {
      e.preventDefault();
      e.stopPropagation();
      clearImagePress();
      return;
    }
    const target = imageTarget(e);
    if (!target || !e.metaKey) return;
    e.preventDefault();
    e.stopPropagation();
    openImageTarget(target);
  };

  const handlePointerDown = (e: ReactPointerEvent): void => {
    const target = imageTarget(e);
    if (!target || e.pointerType === "mouse") return;
    clearImagePress();
    const press: ImagePress = {
      target,
      x: e.clientX,
      y: e.clientY,
      opened: false,
      timer: null,
    };
    press.timer = setTimeout(() => {
      const cur = imagePressRef.current;
      if (!cur || cur.target !== target) return;
      cur.opened = true;
      openImageTarget(target);
    }, IMAGE_LONG_PRESS_MS);
    imagePressRef.current = press;
  };

  const handlePointerMove = (e: ReactPointerEvent): void => {
    const ip = imagePressRef.current;
    if (!ip) return;
    const dx = Math.abs(e.clientX - ip.x);
    const dy = Math.abs(e.clientY - ip.y);
    if (dx > IMAGE_LONG_PRESS_MOVE || dy > IMAGE_LONG_PRESS_MOVE) clearImagePress();
  };

  const handlePointerUp = (): void => {
    const ip = imagePressRef.current;
    if (!ip || ip.opened) return;
    clearImagePress();
  };

  const handlePointerCancel = (): void => {
    clearImagePress();
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
    />
  );
}
