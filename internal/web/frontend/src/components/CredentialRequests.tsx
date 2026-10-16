import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  declineCredRequest,
  loadCredRequest,
  submitCredRequest,
  type CredResult,
} from "../api/client";
import { onPointerDownNoBlur } from "../lib/dom";
import {
  credIDFromURL,
  normalizeCredID,
  removeCredParamFromCurrentURL,
} from "../lib/credIntent";
import type { CredRequest } from "../types/server";

export interface CredentialRequestsProps {
  /** Pending requests from the snapshot (badge source). */
  pending: CredRequest[];
}

type Status = { kind: "idle" | "busy" | "ok" | "err"; text: string };

const IDLE: Status = { kind: "idle", text: "" };

// Events that could carry typed characters to a pane. While the dialog is
// open, any of them aimed outside the dialog is swallowed in the capture
// phase, so a selectPane() that refocuses #direct-input mid-entry cannot send
// the rest of a secret to a live terminal.
const GUARDED_EVENTS = [
  "keydown",
  "keypress",
  "keyup",
  "beforeinput",
  "input",
  "paste",
  "compositionstart",
  "compositionupdate",
  "compositionend",
] as const;

function timeLeft(expiresAt: string | undefined, now: number): string {
  if (!expiresAt) return "";
  const ms = Date.parse(expiresAt) - now;
  if (!Number.isFinite(ms)) return "";
  if (ms <= 0) return "已逾時";
  const s = Math.ceil(ms / 1000);
  const m = Math.floor(s / 60);
  return m > 0 ? `剩 ${m} 分 ${s % 60} 秒` : `剩 ${s} 秒`;
}

function resolvedText(state: string | undefined): string {
  switch (state) {
    case "submitted":
      return "這個請求已經送出過了。";
    case "declined":
      return "這個請求已經被拒絕了。";
    case "expired":
      return "這個請求已經逾時。";
    default:
      return "";
  }
}

export default function CredentialRequests({ pending }: CredentialRequestsProps) {
  const [openID, setOpenID] = useState("");
  const [meta, setMeta] = useState<CredResult | null>(null);
  const [status, setStatus] = useState<Status>(IDLE);
  const [now, setNow] = useState(() => Date.now());
  const cardRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const open = useCallback((rawID: unknown) => {
    const id = normalizeCredID(rawID);
    if (!id) return false;
    setOpenID(id);
    return true;
  }, []);

  const close = useCallback(() => {
    if (inputRef.current) inputRef.current.value = "";
    if (closeTimerRef.current) clearTimeout(closeTimerRef.current);
    closeTimerRef.current = null;
    setOpenID("");
  }, []);

  // Boot deep link: /?cred=<id>.
  useEffect(() => {
    if (open(credIDFromURL(window.location.href))) removeCredParamFromCurrentURL();
  }, [open]);

  // Notification click while a window is already open.
  useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    const onMessage = (event: MessageEvent) => {
      const data: unknown = event.data;
      if (!data || typeof data !== "object") return;
      const msg = data as { type?: unknown; id?: unknown; url?: unknown };
      if (msg.type !== "OPEN_CRED") return;
      open(normalizeCredID(msg.id) || credIDFromURL(String(msg.url || "")));
    };
    navigator.serviceWorker.addEventListener("message", onMessage);
    return () => navigator.serviceWorker.removeEventListener("message", onMessage);
  }, [open]);

  // Load the request's own state; the snapshot only lists pending ones.
  useEffect(() => {
    if (!openID) return;
    let stale = false;
    setMeta(null);
    setStatus(IDLE);
    if (inputRef.current) inputRef.current.value = "";
    loadCredRequest(openID)
      .then(({ res, data }) => {
        if (stale) return;
        if (res.ok) {
          setMeta(data);
          const done = resolvedText(data.state);
          if (done) setStatus({ kind: "err", text: done });
          return;
        }
        setStatus({
          kind: "err",
          text:
            res.status === 404
              ? "找不到這個請求（可能已逾時，或 statusd 重啟過）。"
              : data.error || `讀取失敗（HTTP ${res.status}）`,
        });
      })
      .catch(() => {
        if (!stale) setStatus({ kind: "err", text: "連不上 statusd。" });
      });
    return () => {
      stale = true;
    };
  }, [openID]);

  // Countdown.
  useEffect(() => {
    if (!openID) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [openID]);

  useLayoutEffect(() => {
    if (openID) inputRef.current?.focus();
  }, [openID]);

  // Modal guard: keep focus and keystrokes inside the dialog.
  useEffect(() => {
    if (!openID) return;
    const inside = (target: EventTarget | null) =>
      target instanceof Node && !!cardRef.current?.contains(target);
    const guard = (event: Event) => {
      if (event.type === "keydown" && (event as KeyboardEvent).key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        close();
        return;
      }
      if (inside(event.target)) return;
      event.preventDefault();
      event.stopPropagation();
    };
    const refocus = (event: FocusEvent) => {
      if (!inside(event.target)) inputRef.current?.focus();
    };
    GUARDED_EVENTS.forEach((type) => document.addEventListener(type, guard, true));
    document.addEventListener("focusin", refocus, true);
    return () => {
      GUARDED_EVENTS.forEach((type) => document.removeEventListener(type, guard, true));
      document.removeEventListener("focusin", refocus, true);
    };
  }, [openID, close]);

  useEffect(
    () => () => {
      if (closeTimerRef.current) clearTimeout(closeTimerRef.current);
    },
    [],
  );

  const finish = useCallback(
    async (action: "submit" | "decline") => {
      if (!openID || status.kind === "busy") return;
      let value = "";
      if (action === "submit") {
        value = inputRef.current?.value ?? "";
        if (!value) {
          setStatus({ kind: "err", text: "請先輸入值。" });
          return;
        }
      }
      if (inputRef.current) inputRef.current.value = "";
      setStatus({ kind: "busy", text: action === "submit" ? "送出中…" : "拒絕中…" });
      try {
        const { res, data } =
          action === "submit"
            ? await submitCredRequest(openID, value)
            : await declineCredRequest(openID);
        value = "";
        if (res.ok) {
          setMeta(data);
          setStatus({
            kind: "ok",
            text: action === "submit" ? "已送出，agent 會收到檔案路徑。" : "已拒絕。",
          });
          closeTimerRef.current = setTimeout(close, 1200);
          return;
        }
        setStatus({
          kind: "err",
          text:
            res.status === 409
              ? "這個請求已經處理過或逾時了。"
              : res.status === 404
                ? "找不到這個請求（可能已逾時，或 statusd 重啟過）。"
                : data.error || `失敗（HTTP ${res.status}）`,
        });
      } catch {
        setStatus({ kind: "err", text: "連不上 statusd，請再試一次。" });
      }
    },
    [openID, status.kind, close],
  );

  const sorted = [...pending].sort((a, b) => a.created_at.localeCompare(b.created_at));
  const entry = pending.find((r) => r.id === openID);
  const shown: CredResult | undefined = meta ?? entry;
  const host = shown?.peer || shown?.host || "";
  const others = sorted.filter((r) => r.id !== openID);
  const resolved = !!resolvedText(meta?.state) && status.kind !== "ok";
  const formDisabled =
    status.kind === "busy" || status.kind === "ok" || resolved || (!shown && status.kind === "err");

  return (
    <>
      {!openID && sorted.length > 0 ? (
        <button
          type="button"
          className="cred-badge"
          id="cred-badge"
          onPointerDown={onPointerDownNoBlur}
          onClick={() => open(sorted[0]?.id)}
          aria-label={`${sorted.length} 個待輸入的 credential 請求`}
        >
          🔑 {sorted.length}
        </button>
      ) : null}
      {openID
        ? createPortal(
            <div className="settings-overlay cred-overlay" id="cred-overlay" onClick={close}>
              <div
                className="settings-card cred-card"
                role="dialog"
                aria-modal="true"
                aria-labelledby="cred-title"
                aria-busy={status.kind === "busy"}
                ref={cardRef}
                onClick={(event) => event.stopPropagation()}
              >
                <div className="settings-head">
                  <span id="cred-title">
                    {shown?.name ? `Credential: ${shown.name}` : "Credential request"}
                  </span>
                  <button
                    className="settings-close"
                    type="button"
                    aria-label="close credential request"
                    onPointerDown={onPointerDownNoBlur}
                    onClick={close}
                  >
                    <svg
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2.4"
                      strokeLinecap="round"
                      aria-hidden="true"
                    >
                      <path d="M6 6l12 12M18 6 6 18" />
                    </svg>
                  </button>
                </div>
                <form
                  className="settings-body"
                  autoComplete="off"
                  onSubmit={(event) => {
                    event.preventDefault();
                    void finish("submit");
                  }}
                >
                  {shown ? (
                    <dl className="cred-meta">
                      {shown.reason ? (
                        <>
                          <dt>原因</dt>
                          <dd>{shown.reason}</dd>
                        </>
                      ) : null}
                      {host ? (
                        <>
                          <dt>機器</dt>
                          <dd>{host}</dd>
                        </>
                      ) : null}
                      {shown.pane_id ? (
                        <>
                          <dt>Pane</dt>
                          <dd>{shown.pane_id}</dd>
                        </>
                      ) : null}
                      {shown.cwd ? (
                        <>
                          <dt>目錄</dt>
                          <dd>{shown.cwd}</dd>
                        </>
                      ) : null}
                      {shown.state === "pending" || !shown.state ? (
                        <>
                          <dt>期限</dt>
                          <dd>{timeLeft(shown.expires_at, now)}</dd>
                        </>
                      ) : null}
                    </dl>
                  ) : null}
                  <label className="settings-field">
                    <span>值（存成 0600 檔案，agent 只拿到路徑）</span>
                    <input
                      type="password"
                      id="cred-value"
                      ref={inputRef}
                      autoComplete="off"
                      autoCapitalize="off"
                      autoCorrect="off"
                      spellCheck={false}
                      disabled={formDisabled}
                    />
                  </label>
                  <div className="settings-actions cred-actions">
                    <button
                      type="submit"
                      className="cred-submit"
                      onPointerDown={onPointerDownNoBlur}
                      disabled={formDisabled}
                    >
                      送出
                    </button>
                    <button
                      type="button"
                      className="cred-decline"
                      onPointerDown={onPointerDownNoBlur}
                      onClick={() => void finish("decline")}
                      disabled={formDisabled}
                    >
                      拒絕
                    </button>
                    <span
                      className={`settings-status${
                        status.kind === "ok" ? " ok" : status.kind === "err" ? " err" : ""
                      }`}
                      role="status"
                      aria-live="polite"
                    >
                      {status.text}
                    </span>
                  </div>
                  {others.length > 0 ? (
                    <div className="cred-others">
                      <span>其他待輸入：</span>
                      {others.map((r) => (
                        <button
                          type="button"
                          key={r.id}
                          onPointerDown={onPointerDownNoBlur}
                          onClick={() => open(r.id)}
                        >
                          {r.name}
                          {r.peer || r.host ? ` @ ${r.peer || r.host}` : ""}
                        </button>
                      ))}
                    </div>
                  ) : null}
                </form>
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
