// CopyLineBar — the React port of app.js's "copy as one line" bar
// (#copyline-bar) + wireCopyLine. A multi-line selection of wrapped pane output
// copies with the terminal's soft-wrap newlines + continuation indent baked in;
// the copy actions re-join the selection:
//   joinGlue  — drops the wrap entirely (paths / URLs / commands)
//   joinSpace — collapses each wrap to a single space (prose)
// The run actions instead use smartJoinCommand (post-parity, MIGRATION_SPEC
// item 48a): with the pane grid width from the WS patches, only rows exactly
// pane-width wide (terminal soft-wraps) are glued and real newlines survive,
// so multi-line commands reach `$SHELL -lc` as written. Unknown width falls
// back to joinGlue; the arrow menu's 「接成一行執行」 is the legacy rescue.
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
//   - all action buttons get pointerdown preventDefault (keeps the pane selection
//                alive while the tap is processed — focus would otherwise move
//                to the button and collapse the selection before click runs).
//   - one document `selectionchange` listener drives syncCopyLineBar.
//
// The visible/.copied classes are toggled IMPERATIVELY (matching the original
// classList.toggle) via refs, and copyFlashUntil / the flash timer are refs
// (module-scoped mutable state in the original — never React state).

import { useEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent, RefObject } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import {
  loadCommandJob,
  fetchSnapshot,
  startBackgroundCommand,
  startTmuxCommand,
} from "../api/client";
import type { CommandJob } from "../api/client";
import CommandResultDialog from "./CommandResultDialog";

const FLASH_MS = 900;
const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;

function joinGlue(text: string): string {
  return text.replace(/[ \t]*\n[ \t]*/g, "");
}
function joinSpace(text: string): string {
  return text.replace(/[ \t]*\n[ \t]*/g, " ");
}

// displayColumns approximates the tmux grid width of a captured row: CJK /
// fullwidth code points occupy two columns, zero-width marks none. It only has
// to reproduce tmux's count well enough that "row exactly pane-width wide"
// detection holds; a miscount degrades to keeping the newline.
export function displayColumns(line: string): number {
  let cols = 0;
  for (const ch of line) {
    const cp = ch.codePointAt(0) ?? 0;
    if (
      cp === 0x200b ||
      cp === 0x200d ||
      (cp >= 0xfe00 && cp <= 0xfe0f) ||
      (cp >= 0x0300 && cp <= 0x036f)
    ) {
      continue;
    }
    const wide =
      (cp >= 0x1100 && cp <= 0x115f) ||
      (cp >= 0x2e80 && cp <= 0xa4cf) ||
      (cp >= 0xac00 && cp <= 0xd7a3) ||
      (cp >= 0xf900 && cp <= 0xfaff) ||
      (cp >= 0xfe30 && cp <= 0xfe4f) ||
      (cp >= 0xff00 && cp <= 0xff60) ||
      (cp >= 0xffe0 && cp <= 0xffe6) ||
      (cp >= 0x1f300 && cp <= 0x1f64f) ||
      (cp >= 0x1f900 && cp <= 0x1faff) ||
      (cp >= 0x20000 && cp <= 0x3fffd);
    cols += wide ? 2 : 1;
  }
  return cols;
}

// smartJoinCommand re-joins a selection [start,end) of the rendered pane text
// into a runnable shell command. Captured rows exactly paneWidth columns wide
// are terminal soft-wraps of one logical line, so their trailing newline is
// glued away verbatim; every other newline is a real line break and is kept —
// the run endpoints hand the whole string to `$SHELL -lc`, which handles
// multi-line scripts and trailing-backslash continuations natively. The full
// buffer text is needed (not just the selection) so a selection that starts
// mid-row still measures the entire grid row.
export function smartJoinCommand(
  fullText: string,
  start: number,
  end: number,
  paneWidth: number,
): string {
  let out = "";
  let i = start;
  while (i < end) {
    const nl = fullText.indexOf("\n", i);
    if (nl === -1 || nl >= end) {
      out += fullText.slice(i, end);
      break;
    }
    out += fullText.slice(i, nl);
    const lineStart = fullText.lastIndexOf("\n", nl - 1) + 1;
    if (displayColumns(fullText.slice(lineStart, nl)) !== paneWidth) {
      out += "\n";
    }
    i = nl + 1;
  }
  return out.trim();
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

// paneSelectionContext locates the live pane selection inside the full
// rendered buffer text so smartJoinCommand can measure whole grid rows. Null
// when there is no pane selection or when Range-based offsets do not line up
// with the selection text (markdown mode's block rendering breaks the
// equivalence) — callers then fall back to the legacy glue join.
function paneSelectionContext(): { fullText: string; start: number; end: number } | null {
  const sel = window.getSelection();
  if (!sel || sel.isCollapsed || sel.rangeCount === 0) return null;
  const content = document.getElementById("content");
  if (!content) return null;
  if (!content.contains(sel.anchorNode) || !content.contains(sel.focusNode)) return null;
  const range = sel.getRangeAt(0);
  const full = document.createRange();
  full.selectNodeContents(content);
  const before = full.cloneRange();
  before.setEnd(range.startContainer, range.startOffset);
  const start = before.toString().length;
  const selText = range.toString();
  const fullText = full.toString();
  if (!selText || fullText.slice(start, start + selText.length) !== selText) return null;
  return { fullText, start, end: start + selText.length };
}

export interface CopyLineBarProps {
  cwd?: string | null;
  peer?: string | null;
  paneID?: string | null;
  /**
   * Latest streamed pane grid width in columns (0 = unknown). Read at click
   * time so patches don't need to re-render the bar. Unknown width falls back
   * to the legacy join-everything command build.
   */
  getPaneWidth?: () => number;
  onSelectPane?: (paneID: string) => void;
  onError?: (message: string) => void;
  startBackground?: typeof startBackgroundCommand;
  loadJob?: typeof loadCommandJob;
  startTmux?: typeof startTmuxCommand;
  waitForPane?: (paneID: string) => Promise<void>;
}

async function waitForPaneSnapshot(paneID: string): Promise<void> {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      const snapshot = await fetchSnapshot();
      if (Object.values(snapshot.panes || {}).some((pane) => pane.pane_id === paneID)) return;
    } catch {
      // The normal snapshot stream may still be reconnecting; retry briefly.
    }
    await new Promise((resolve) => window.setTimeout(resolve, 200));
  }
}

export default function CopyLineBar({
  cwd,
  peer,
  paneID,
  getPaneWidth,
  onSelectPane,
  onError,
  startBackground = startBackgroundCommand,
  loadJob = loadCommandJob,
  startTmux = startTmuxCommand,
  waitForPane = waitForPaneSnapshot,
}: CopyLineBarProps) {
  const barRef = useRef<HTMLDivElement | null>(null);
  const joinRef = useRef<HTMLButtonElement | null>(null);
  const spaceRef = useRef<HTMLButtonElement | null>(null);
  const downloadRef = useRef<HTMLAnchorElement | null>(null);
  const runRef = useRef<HTMLButtonElement | null>(null);
  const runMenuCommandRef = useRef("");
  const runMenuGluedRef = useRef("");
  const runMenuOpenRef = useRef(false);
  const commandJobPeerRef = useRef<string | undefined>(undefined);
  const commandRequestRef = useRef(0);
  const [runMenuOpen, setRunMenuOpen] = useState(false);
  const [commandJob, setCommandJob] = useState<CommandJob | null>(null);
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
    const has =
      selection.trim().length > 0 ||
      Date.now() < copyFlashUntil.current ||
      runMenuOpenRef.current;
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

  useEffect(() => {
    if (!runMenuOpen) return;
    const close = (): void => {
      runMenuOpenRef.current = false;
      setRunMenuOpen(false);
      syncCopyLineBar.current();
    };
    const onKey = (event: KeyboardEvent): void => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("click", close);
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", onKey, true);
    };
  }, [runMenuOpen]);

  useEffect(() => {
    if (!commandJob?.id || commandJob.status !== "running") return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const poll = async (): Promise<void> => {
      try {
        const { res, data } = await loadJob(commandJob.id, commandJobPeerRef.current);
        if (stopped) return;
        if (!res.ok || !data.job) {
          const finished = new Date().toISOString();
          setCommandJob((current) =>
            current
              ? {
                  ...current,
                  status: "finished",
                  error: data.error || `HTTP ${res.status}`,
                  exit_code: -1,
                  finished_at: finished,
                }
              : null,
          );
          return;
        }
        setCommandJob(data.job);
        if (data.job.status === "running") timer = setTimeout(poll, 350);
      } catch (error) {
        if (stopped) return;
        setCommandJob((current) =>
          current
            ? {
                ...current,
                status: "finished",
                error: error instanceof Error ? error.message : String(error),
                exit_code: -1,
                finished_at: new Date().toISOString(),
              }
            : null,
        );
      }
    };
    timer = setTimeout(poll, 250);
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [commandJob?.id, commandJob?.status, loadJob]);

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
      runRef.current?.classList.remove("copied");
      btnRef.current?.classList.add("copied");
      syncCopyLineBar.current();
      if (flashTimer.current) clearTimeout(flashTimer.current);
      flashTimer.current = setTimeout(() => {
        btnRef.current?.classList.remove("copied");
        syncCopyLineBar.current();
      }, FLASH_MS);
    };

  // selectedCommand builds the runnable command from the live selection. With
  // a known pane width it only glues true terminal soft-wraps and keeps real
  // newlines (the run endpoints execute via `$SHELL -lc`, which takes
  // multi-line scripts as written); without one it falls back to the legacy
  // glue-everything join.
  const selectedCommand = (): string => {
    const width = getPaneWidth?.() ?? 0;
    if (width > 0) {
      const context = paneSelectionContext();
      if (context) {
        return smartJoinCommand(context.fullText, context.start, context.end, width);
      }
    }
    return joinGlue(paneSelectionText());
  };

  const runCommand = async (command?: string): Promise<void> => {
    if (command === undefined) command = selectedCommand();
    if (!command || !paneID) return;
    const request = ++commandRequestRef.current;
    commandJobPeerRef.current = paneID.includes("@")
      ? paneID.slice(0, paneID.lastIndexOf("@"))
      : undefined;
    setCommandJob({
      id: "",
      status: "running",
      command,
      started_at: new Date().toISOString(),
    });
    try {
      const { res, data } = await startBackground(paneID, command);
      if (request !== commandRequestRef.current) return;
      if (!res.ok || !data.job) {
        setCommandJob((current) =>
          current
            ? {
                ...current,
                status: "finished",
                error: data.error || `HTTP ${res.status}`,
                exit_code: -1,
                finished_at: new Date().toISOString(),
              }
            : null,
        );
        return;
      }
      setCommandJob(data.job);
    } catch (error) {
      if (request !== commandRequestRef.current) return;
      setCommandJob((current) =>
        current
          ? {
              ...current,
              status: "finished",
              error: error instanceof Error ? error.message : String(error),
              exit_code: -1,
              finished_at: new Date().toISOString(),
            }
          : null,
      );
    }
  };

  const toggleRunMenu = (event: ReactMouseEvent): void => {
    event.stopPropagation();
    // Capture both variants while the selection is still alive: the smart join
    // for "Run in tmux session" and the legacy glue as an explicit rescue for
    // soft-wraps the width heuristic cannot see (e.g. an agent TUI's own
    // word-wrap at less than the pane width).
    const text = paneSelectionText();
    if (text) {
      runMenuCommandRef.current = selectedCommand();
      runMenuGluedRef.current = joinGlue(text);
    }
    if (!runMenuCommandRef.current && !runMenuGluedRef.current) return;
    const next = !runMenuOpenRef.current;
    runMenuOpenRef.current = next;
    setRunMenuOpen(next);
    syncCopyLineBar.current();
  };

  const runGluedFromMenu = (event: ReactMouseEvent): void => {
    event.stopPropagation();
    const command = runMenuGluedRef.current;
    runMenuOpenRef.current = false;
    setRunMenuOpen(false);
    syncCopyLineBar.current();
    if (!command) return;
    void runCommand(command);
  };

  const runInTmux = async (event: ReactMouseEvent): Promise<void> => {
    event.stopPropagation();
    const command = runMenuCommandRef.current;
    runMenuOpenRef.current = false;
    setRunMenuOpen(false);
    syncCopyLineBar.current();
    if (!command || !paneID) return;
    try {
      const { res, data } = await startTmux(paneID, command);
      if (!res.ok || !data.pane_id) {
        onError?.(data.error || `HTTP ${res.status}`);
        return;
      }
      const nextPane = peer ? `${peer}@${data.pane_id}` : data.pane_id;
      await waitForPane(nextPane);
      onSelectPane?.(nextPane);
    } catch (error) {
      onError?.(error instanceof Error ? error.message : String(error));
    }
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
      <div className="copyline-run-group">
        <button
          className="copyline-btn alt copyline-run-main"
          id="copyline-run"
          type="button"
          title="在背景執行選取的指令並顯示輸出"
          ref={runRef}
          onPointerDown={onPointerDownNoBlur}
          onClick={() => void runCommand()}
        >
          Run command
        </button>
        <button
          className="copyline-btn alt copyline-run-arrow"
          id="copyline-run-arrow"
          type="button"
          title="更多執行方式"
          aria-label="more command run options"
          aria-haspopup="menu"
          aria-expanded={runMenuOpen}
          onPointerDown={onPointerDownNoBlur}
          onClick={toggleRunMenu}
        >
          <svg viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
            <path d="M2 4l4 4 4-4z" />
          </svg>
        </button>
        {runMenuOpen ? (
          <div className="copyline-run-menu" role="menu" onClick={(event) => event.stopPropagation()}>
            <button
              type="button"
              role="menuitem"
              onPointerDown={onPointerDownNoBlur}
              onClick={(event) => void runInTmux(event)}
            >
              Run in tmux session
            </button>
            <button
              type="button"
              role="menuitem"
              title="把選取內容的換行全部去掉、接成一行後執行(寬度判斷失準時的救援)"
              onPointerDown={onPointerDownNoBlur}
              onClick={runGluedFromMenu}
            >
              接成一行執行
            </button>
          </div>
        ) : null}
      </div>
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
      <CommandResultDialog
        job={commandJob}
        onClose={() => {
          commandRequestRef.current += 1;
          setCommandJob(null);
        }}
      />
    </div>
  );
}
