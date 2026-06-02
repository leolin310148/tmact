// MarkdownToggle — a content-pane FAB that flips the pane output between the
// raw ANSI terminal view and the "markdown view" (pipe-delimited tables folded
// into real <table>s; see terminal/render.ts extractPipeTables). Unlike the
// upload/selection/clear FABs — whose visibility/disabled state App mutates
// imperatively (syncQuickDock) — this button is wholly React-owned: it shows
// only when a pane is selected and reflects the persisted markdownView flag via
// the `.active` class. Anchored bottom-LEFT so it never collides with the
// right-side FAB cluster, qb-dock, or the copy-line bar.
//
// pointerdown preventDefault (onPointerDownNoBlur) keeps the mobile soft
// keyboard from dropping when the FAB is tapped, matching the other pane FABs.

import { onPointerDownNoBlur } from "../lib/dom";

export interface MarkdownToggleProps {
  /** Shown only when a pane is selected (there is output to reinterpret). */
  visible: boolean;
  /** Reflects the persisted markdownView flag (`.active` highlight). */
  active: boolean;
  /** Flip raw ⇄ markdown (App persists + re-renders ContentPane). */
  onToggle: () => void;
}

export default function MarkdownToggle({ visible, active, onToggle }: MarkdownToggleProps) {
  return (
    <button
      className={"markdown-btn" + (visible ? " ready" : "") + (active ? " active" : "")}
      id="markdown-btn"
      type="button"
      title="切換 markdown 表格檢視 / 原始輸出"
      aria-label="toggle markdown table view"
      aria-pressed={active ? "true" : "false"}
      onPointerDown={onPointerDownNoBlur}
      onClick={onToggle}
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
        <rect x="3" y="5" width="18" height="14" rx="1.5" />
        <path d="M3 10h18" />
        <path d="M9 10v9" />
      </svg>
    </button>
  );
}
