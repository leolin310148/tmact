import { useEffect, useLayoutEffect, useRef } from "react";
import { createPortal } from "react-dom";
import type { CommandJob } from "../api/client";

export interface CommandResultDialogProps {
  job: CommandJob | null;
  onClose: () => void;
}

export default function CommandResultDialog({ job, onClose }: CommandResultDialogProps) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const invokingControlRef = useRef<HTMLElement | null>(null);
  const wasOpenRef = useRef(false);

  useLayoutEffect(() => {
    if (job) {
      if (!wasOpenRef.current) {
        const active = document.activeElement;
        if (active instanceof HTMLElement) invokingControlRef.current = active;
        closeRef.current?.focus();
      }
      wasOpenRef.current = true;
      return;
    }
    if (wasOpenRef.current) {
      wasOpenRef.current = false;
      const invokingControl = invokingControlRef.current;
      invokingControlRef.current = null;
      if (invokingControl?.isConnected) invokingControl.focus();
    }
  }, [job]);

  useEffect(() => {
    if (!job) return;
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      onClose();
    };
    document.addEventListener("keydown", onKeyDown, true);
    return () => document.removeEventListener("keydown", onKeyDown, true);
  }, [job, onClose]);

  if (!job) return null;

  const running = job.status === "running";
  const failed = !running && (job.error || (job.exit_code ?? 0) !== 0);
  const status = running
    ? "執行中…"
    : job.error
      ? "無法執行"
      : `已完成 · exit ${job.exit_code ?? 0}`;

  return createPortal(
    <div className="command-result-overlay" id="command-result-overlay" onClick={onClose}>
      <div
        className="settings-card command-result-card"
        role="dialog"
        aria-modal="true"
        aria-labelledby="command-result-title"
        aria-busy={running}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          event.preventDefault();
          closeRef.current?.focus();
        }}
      >
        <div className="settings-head">
          <span id="command-result-title">Command output</span>
          <button
            className="settings-close"
            type="button"
            aria-label="close command output"
            onClick={onClose}
            ref={closeRef}
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
        <div className="command-result-command">{job.command}</div>
        <div
          className={`command-result-status${failed ? " failed" : ""}`}
          role="status"
          aria-live="polite"
        >
          {status}
        </div>
        <pre className="command-result-output">
          {job.output || (running ? "等待指令完成…" : "(沒有輸出)")}
        </pre>
        {job.truncated ? (
          <div className="command-result-truncated" role="note">
            輸出過長，僅顯示前 2 MiB。
          </div>
        ) : null}
        {job.error ? <div className="command-result-error">{job.error}</div> : null}
      </div>
    </div>,
    document.body,
  );
}
