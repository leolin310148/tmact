// OverflowMenu — the shared "more" popover used by BOTH pane-switcher layouts
// (the statusline's MoreChip and the office layout's floor-lamp trigger). One
// implementation owns:
//   - useMenuPopover: open/close state, outside-pointerdown + Escape dismissal,
//     keyboard traversal (roving [role=menuitem] focus) and trigger-focus
//     restore — previously duplicated in MoreChip and OfficeDesks.LampMore.
//   - OverflowMenuContent: the rich row list. Each hidden pane renders a state
//     dot + peer badge + label + runtime badge plus an exit button that kills
//     the whole tmux session after a two-stage confirm (first press arms it,
//     second press kills; it disarms on a timeout). Below the panes, a
//     "recently closed" section lists sessions from /api/sessions/closed —
//     selecting one recreates it (same name, old cwd, plain shell).
//
// Session kill/reopen calls are made here (not lifted to App): failures render
// as an inline error line at the foot of the menu, and success needs no
// handling — the next snapshot poll updates the pane list naturally.

import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import {
  killSession,
  loadClosedSessions,
  reopenSession,
  reportHumanActivity,
} from "../api/client";
import { focusMenuEdge, moveMenuFocus, onPointerDownNoBlur } from "../lib/dom";
import type { ClosedSession } from "../types/server";
import {
  RUNTIME_ICON,
  panePeer,
  paneRuntime,
  paneStateClass,
  paneStateLabel,
  sessionLabel,
  type PaneListItem,
} from "./StatusLine";

// useMenuPopover owns one popover-menu's lifecycle. `menuReady` is any value
// that changes when the menu element appears in the DOM (the office trigger
// portals its menu only after measuring the anchor, so the pending keyboard
// focus must re-run then); pass `open` itself when the menu mounts with it.
export function useMenuPopover(menuReady: unknown) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const pendingFocusRef = useRef<"first" | "last" | null>(null);

  const openFromKeyboard = (edge: "first" | "last") => {
    pendingFocusRef.current = edge;
    setOpen(true);
  };

  useLayoutEffect(() => {
    if (!open || !pendingFocusRef.current || !menuRef.current) return;
    focusMenuEdge(menuRef.current, pendingFocusRef.current);
    pendingFocusRef.current = null;
  }, [open, menuReady]);

  // Close on outside pointerdown (capture so it fires before the row click)
  // and on Escape, but only while open.
  useEffect(() => {
    if (!open) return;
    const onDocPointerDown = (e: Event) => {
      const t = e.target as Node;
      if (buttonRef.current?.contains(t) || menuRef.current?.contains(t)) return;
      setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        const restoreTrigger = menuRef.current?.contains(document.activeElement) ?? false;
        pendingFocusRef.current = null;
        setOpen(false);
        if (restoreTrigger) buttonRef.current?.focus();
      }
    };
    document.addEventListener("pointerdown", onDocPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onDocPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const onTriggerClick = () => {
    pendingFocusRef.current = null;
    setOpen((v) => !v);
  };

  const onTriggerKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      openFromKeyboard(e.key === "ArrowUp" ? "last" : "first");
      return;
    }
    if (!open && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      openFromKeyboard("first");
    }
  };

  const onMenuKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (!moveMenuFocus(e.currentTarget, e.key)) return;
    e.preventDefault();
  };

  // closeRestoring closes the menu after a row action, returning focus to the
  // trigger when focus was inside the menu (keyboard flow).
  const closeRestoring = () => {
    const restoreTrigger = menuRef.current?.contains(document.activeElement) ?? false;
    setOpen(false);
    if (restoreTrigger) buttonRef.current?.focus();
  };

  return {
    open,
    setOpen,
    buttonRef,
    menuRef,
    onTriggerClick,
    onTriggerKeyDown,
    onMenuKeyDown,
    closeRestoring,
  };
}

// shortCwd compresses an absolute cwd to its two tail segments for the closed
// row's dim path hint ("…/puni/tmact").
function shortCwd(cwd: string): string {
  const parts = cwd.split("/").filter(Boolean);
  if (parts.length <= 2) return cwd;
  return "…/" + parts.slice(-2).join("/");
}

// closedKey identifies one closed entry across peers.
function closedKey(c: ClosedSession): string {
  return (c.peer ?? "") + "\0" + c.session;
}

interface OverflowMenuContentProps {
  /** The collapsed (agent-less, idle) panes to list. */
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
  /** Close the popover after a row selection (restores trigger focus). */
  closeRestoring: () => void;
}

export function OverflowMenuContent({ items, onSelect, closeRestoring }: OverflowMenuContentProps) {
  const [closed, setClosed] = useState<ClosedSession[] | null>(null);
  const [error, setError] = useState("");
  // armed = pane row whose exit button awaits its confirming second press.
  const [armed, setArmed] = useState<string | null>(null);
  // busy = session keys with an in-flight kill/reopen (rows render dimmed).
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const disarmTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // The content only exists while the popover is open, so fetch-on-mount is
  // fetch-on-open. History is best-effort: failures leave the section empty.
  useEffect(() => {
    let stale = false;
    loadClosedSessions()
      .then(({ res, data }) => {
        if (stale) return;
        if (!res.ok) {
          setClosed([]);
          return;
        }
        setClosed(data.sessions ?? []);
      })
      .catch(() => {
        if (!stale) setClosed([]);
      });
    return () => {
      stale = true;
    };
  }, []);

  useEffect(() => () => {
    if (disarmTimer.current) clearTimeout(disarmTimer.current);
  }, []);

  const arm = (key: string) => {
    setArmed(key);
    if (disarmTimer.current) clearTimeout(disarmTimer.current);
    disarmTimer.current = setTimeout(() => setArmed(null), 3000);
  };

  const setKeyBusy = (key: string, value: boolean) => {
    setBusy((prev) => {
      const next = { ...prev };
      if (value) next[key] = true;
      else delete next[key];
      return next;
    });
  };

  const doKill = (item: PaneListItem, key: string) => {
    const session = sessionLabel(item.pane);
    const peer = panePeer(item.pane) || undefined;
    setArmed(null);
    setError("");
    setKeyBusy(key, true);
    reportHumanActivity();
    killSession(session, peer)
      .then(({ res, data }) => {
        if (!res.ok) setError("exit " + session + ": " + (data.error || "HTTP " + res.status));
      })
      .catch((e) => setError("exit " + session + ": " + (e instanceof Error ? e.message : "failed")))
      .finally(() => setKeyBusy(key, false));
  };

  const doReopen = (entry: ClosedSession) => {
    const key = closedKey(entry);
    setError("");
    setKeyBusy(key, true);
    reportHumanActivity();
    reopenSession(entry.session, entry.cwd ?? "", entry.peer || undefined)
      .then(({ res, data }) => {
        if (!res.ok) {
          setError("reopen " + entry.session + ": " + (data.error || "HTTP " + res.status));
          return;
        }
        // The reopened session leaves history immediately; its pane appears
        // with the next snapshot poll.
        setClosed((prev) => (prev ?? []).filter((c) => closedKey(c) !== key));
      })
      .catch((e) =>
        setError("reopen " + entry.session + ": " + (e instanceof Error ? e.message : "failed")),
      )
      .finally(() => setKeyBusy(key, false));
  };

  const paneRowKey = (item: PaneListItem, i: number) => item.pane.pane_id || "overflow-" + i;

  return (
    <>
      {items.length === 0 && (closed?.length ?? 0) === 0 ? (
        <div className="ovf-empty">{closed === null ? "loading…" : "no hidden panes"}</div>
      ) : null}
      {items.map((item, i) => {
        const { pane, label } = item;
        const key = paneRowKey(item, i);
        const runtime = paneRuntime(pane);
        const peer = panePeer(pane);
        const icon = RUNTIME_ICON[runtime];
        const isArmed = armed === key;
        const isBusy = !!busy[key];
        const title =
          (peer ? peer + " — " : "") +
          (pane.cwd || pane.session) +
          " — " +
          (runtime || "idle") +
          " — " +
          paneStateLabel(pane);
        return (
          <div
            key={key}
            className={
              "ovf-row state-" + paneStateClass(pane) + (isBusy ? " busy" : "")
            }
          >
            <button
              type="button"
              className="ovf-main"
              role="menuitem"
              tabIndex={-1}
              title={title}
              aria-label={"Select pane " + (peer ? peer + " " : "") + label}
              disabled={isBusy}
              onPointerDown={onPointerDownNoBlur}
              onClick={() => {
                onSelect(pane.pane_id ?? "");
                closeRestoring();
              }}
              onKeyDown={(e) => {
                // Delete/Backspace mirrors the exit button for keyboard users.
                if (e.key !== "Delete" && e.key !== "Backspace") return;
                e.preventDefault();
                if (isArmed) doKill(item, key);
                else arm(key);
              }}
            >
              <span className="ovf-dot" aria-hidden="true" />
              {peer ? <span className="peer-badge">{peer}</span> : null}
              <span className="ovf-label">{label}</span>
              {icon ? <span className="ovf-rt">{icon}</span> : null}
            </button>
            <button
              type="button"
              tabIndex={-1}
              className={"ovf-kill" + (isArmed ? " armed" : "")}
              title={
                isArmed
                  ? "Press again to kill session " + sessionLabel(pane)
                  : "Close session " + sessionLabel(pane)
              }
              aria-label={
                (isArmed ? "Confirm closing session " : "Close session ") + sessionLabel(pane)
              }
              disabled={isBusy}
              onPointerDown={onPointerDownNoBlur}
              onClick={() => {
                if (isArmed) doKill(item, key);
                else arm(key);
              }}
            >
              {isArmed ? "kill?" : "✕"}
            </button>
          </div>
        );
      })}
      {closed && closed.length > 0 ? (
        <>
          <div className="ovf-sep" role="separator">
            recently closed
          </div>
          {closed.map((entry) => {
            const key = closedKey(entry);
            const icon = RUNTIME_ICON[(entry.runtime ?? "").toLowerCase()];
            const isBusy = !!busy[key];
            return (
              <div key={key} className={"ovf-row ovf-closed" + (isBusy ? " busy" : "")}>
                <button
                  type="button"
                  className="ovf-main"
                  role="menuitem"
                  tabIndex={-1}
                  title={
                    "Reopen session " +
                    entry.session +
                    (entry.cwd ? " at " + entry.cwd : "") +
                    (entry.peer ? " on " + entry.peer : "")
                  }
                  aria-label={"Reopen session " + entry.session}
                  disabled={isBusy}
                  onPointerDown={onPointerDownNoBlur}
                  onClick={() => doReopen(entry)}
                >
                  <span className="ovf-reopen" aria-hidden="true">
                    ↺
                  </span>
                  {entry.peer ? <span className="peer-badge">{entry.peer}</span> : null}
                  <span className="ovf-label">{entry.session}</span>
                  {entry.cwd ? <span className="ovf-cwd">{shortCwd(entry.cwd)}</span> : null}
                  {icon ? <span className="ovf-rt">{icon}</span> : null}
                </button>
              </div>
            );
          })}
        </>
      ) : null}
      {error ? <div className="ovf-error">{error}</div> : null}
    </>
  );
}
