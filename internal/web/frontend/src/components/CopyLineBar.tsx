// CopyLineBar — the React port of app.js's "copy as one line" bar
// (#copyline-bar) + wireCopyLine. A multi-line selection of wrapped pane output
// copies with the terminal's soft-wrap newlines + continuation indent baked in;
// these two buttons re-join the selection:
//   joinGlue  — drops the wrap entirely (paths / URLs / commands)
//   joinSpace — collapses each wrap to a single space (prose)
//
// Parity notes (byte-for-behavior with app.js lines 920–1012):
//   - joinGlue:  /[ \t]*\n[ \t]*/g -> ""
//   - joinSpace: /[ \t]*\n[ \t]*/g -> " "
//   - copyText:  navigator.clipboard.writeText if window.isSecureContext, else a
//                hidden fixed-position textarea + document.execCommand("copy").
//   - paneSelectionText: the live selection, only when non-collapsed AND both
//                anchorNode/focusNode are inside #content.
//   - visible when paneSelectionText().trim() non-empty OR Date.now() <
//                copyFlashUntil (the 900 ms green flash).
//   - both buttons get pointerdown preventDefault (keeps the pane selection
//                alive while the tap is processed — focus would otherwise move
//                to the button and collapse the selection before click runs).
//   - one document `selectionchange` listener drives syncCopyLineBar.
//
// The visible/.copied classes are toggled IMPERATIVELY (matching the original
// classList.toggle) via refs, and copyFlashUntil / the flash timer are refs
// (module-scoped mutable state in the original — never React state).

import { useEffect, useRef } from "react";
import type { RefObject } from "react";
import { onPointerDownNoBlur } from "../lib/dom";

const FLASH_MS = 900;
const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;

function joinGlue(text: string): string {
  return text.replace(/[ \t]*\n[ \t]*/g, "");
}
function joinSpace(text: string): string {
  return text.replace(/[ \t]*\n[ \t]*/g, " ");
}

function unquotePath(text: string): string {
  const t = text.trim();
  if (!isQuotedPath(t)) return t;
  return t.slice(1, -1).trim();
}

function isQuotedPath(t: string): boolean {
  if (t.length < 2) return false;
  const first = t[0];
  const last = t[t.length - 1];
  return (first === '"' && last === '"') || (first === "'" && last === "'") || (first === "`" && last === "`");
}

export function selectedDownloadPath(text: string): string {
  const raw = joinGlue(text).trim();
  const quoted = isQuotedPath(raw);
  const path = unquotePath(raw);
  if (!path || /[\r\n\x00]/.test(path)) return "";
  const scheme = URL_SCHEME_RE.exec(path);
  if (scheme && scheme[0].toLowerCase() !== "file://") return "";
  if (path.startsWith("file://") || path.startsWith("/") || path.startsWith("./") || path.startsWith("../")) {
    return path;
  }
  if (!quoted && /\s/.test(path)) return "";
  if (path.includes("/")) return path;
  if (/\.[A-Za-z0-9][A-Za-z0-9._-]*$/.test(path)) return path;
  return "";
}

export function buildFileDownloadHref(path: string, cwd?: string | null, peer?: string | null): string {
  const qs = new URLSearchParams({ path });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  return "/api/file?" + qs.toString();
}

// copyText writes to the clipboard, falling back to a hidden-textarea
// execCommand for plain-http origins where navigator.clipboard is unavailable
// (the statusd web UI is usually served over a LAN IP, not https/localhost).
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    /* fall through to legacy path */
  }
  try {
    const ta = document.createElement("textarea");
    ta.readOnly = true;
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, text.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

// paneSelectionText returns the current selection only when it is non-empty and
// anchored inside the pane output, so a selection in the draft box (or anywhere
// else) never triggers the bar.
function paneSelectionText(): string {
  const sel = window.getSelection();
  if (!sel || sel.isCollapsed) return "";
  const content = document.getElementById("content");
  if (!content) return "";
  if (!content.contains(sel.anchorNode) || !content.contains(sel.focusNode)) return "";
  return sel.toString();
}

export interface CopyLineBarProps {
  cwd?: string | null;
  peer?: string | null;
}

export default function CopyLineBar({ cwd, peer }: CopyLineBarProps) {
  const barRef = useRef<HTMLDivElement | null>(null);
  const joinRef = useRef<HTMLButtonElement | null>(null);
  const spaceRef = useRef<HTMLButtonElement | null>(null);
  const downloadRef = useRef<HTMLAnchorElement | null>(null);
  const copyFlashUntil = useRef(0);
  const flashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // syncCopyLineBar — toggles .visible + aria-hidden, exactly as app.js. Held in
  // a ref so the stable selectionchange listener always calls the latest copy.
  const syncCopyLineBar = useRef<() => void>(() => {});
  syncCopyLineBar.current = () => {
    const bar = barRef.current;
    if (!bar) return;
    const selection = paneSelectionText();
    const downloadPath = selectedDownloadPath(selection);
    if (downloadRef.current) {
      downloadRef.current.hidden = !downloadPath;
      if (downloadPath) downloadRef.current.href = buildFileDownloadHref(downloadPath, cwd, peer);
      else downloadRef.current.removeAttribute("href");
    }
    const has = selection.trim().length > 0 || Date.now() < copyFlashUntil.current;
    bar.classList.toggle("visible", has);
    bar.setAttribute("aria-hidden", has ? "false" : "true");
  };

  // One listener covers desktop drag-select, mobile selection-mode handles, and
  // selection loss on pane re-render / pane switch. Registered on mount and the
  // initial sync run, mirroring wireCopyLine's tail.
  useEffect(() => {
    const handler = (): void => syncCopyLineBar.current();
    document.addEventListener("selectionchange", handler);
    syncCopyLineBar.current();
    return () => {
      document.removeEventListener("selectionchange", handler);
      if (flashTimer.current) clearTimeout(flashTimer.current);
    };
  }, [cwd, peer]);

  const run = (btnRef: RefObject<HTMLButtonElement | null>, transform: (t: string) => string) =>
    async (): Promise<void> => {
      const text = paneSelectionText();
      if (!text) return;
      if (!(await copyText(transform(text)))) return;
      // Hold the bar up briefly so the green "copied" flash is visible even when
      // the execCommand fallback collapsed the pane selection.
      copyFlashUntil.current = Date.now() + FLASH_MS;
      joinRef.current?.classList.remove("copied");
      spaceRef.current?.classList.remove("copied");
      btnRef.current?.classList.add("copied");
      syncCopyLineBar.current();
      if (flashTimer.current) clearTimeout(flashTimer.current);
      flashTimer.current = setTimeout(() => {
        btnRef.current?.classList.remove("copied");
        syncCopyLineBar.current();
      }, FLASH_MS);
    };

  return (
    <div className="copyline-bar" id="copyline-bar" aria-hidden="true" ref={barRef}>
      <button
        className="copyline-btn"
        id="copyline-join"
        type="button"
        title="複製選取內容、把斷行接成一行(去掉換行與續行縮排)"
        ref={joinRef}
        onPointerDown={onPointerDownNoBlur}
        onClick={run(joinRef, joinGlue)}
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
          <path d="M3 6h18" />
          <path d="M3 12h12" />
          <path d="M3 18h18" />
          <path d="m17 9 4 3-4 3" />
        </svg>
        <span>複製成一行</span>
      </button>
      <button
        className="copyline-btn alt"
        id="copyline-space"
        type="button"
        title="複製選取內容、斷行以單一空白接合(適合一般文字)"
        ref={spaceRef}
        onPointerDown={onPointerDownNoBlur}
        onClick={run(spaceRef, joinSpace)}
      >
        接空白
      </button>
      <a
        className="copyline-btn alt"
        id="copyline-download"
        title="下載選取的檔案路徑"
        aria-label="download selected file"
        hidden
        ref={downloadRef}
        onPointerDown={onPointerDownNoBlur}
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
          <path d="M12 3v12" />
          <path d="m7 10 5 5 5-5" />
          <path d="M5 21h14" />
        </svg>
        <span>下載</span>
      </a>
    </div>
  );
}
