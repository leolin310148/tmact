// The small content-wrap action buttons (help / upload / selection / clear)
// plus the hidden file input. 1:1 markup port of the buttons in
// static/index.html that app.js / quick.js / help.js wire by id.
//
// These buttons keep IDENTICAL ids/classes/SVG so app.css applies and so App's
// imperative sync code (syncQuickDock toggling `.ready`/`disabled` on
// #upload-btn/#selection-btn/#clear-pane-btn, selectPane enabling #upload-btn)
// can still reach them via getElementById. React must NOT own their
// disabled/aria-pressed state — App mutates them imperatively, exactly as the
// original did. We therefore render the static initial attributes from
// index.html (all `disabled`; selection `aria-pressed="false"`).
//
// onPointerDownNoBlur audit (ARCHITECTURE.md §6): selection / clear-pane / help
// get pointerdown preventDefault; #upload-btn does NOT (app.js wires no
// preventDefault on upload-btn). The help button's click also stopPropagation()s
// before toggling, matching help.js.

import { onPointerDownNoBlur } from "../lib/dom";

export interface UploadControlsProps {
  /** #upload-btn click → openFileUploadPicker. */
  onUpload: () => void;
  /** #selection-btn click → toggleSelectionMode. */
  onSelection: () => void;
  /** #clear-pane-btn click → clearPaneOutput. */
  onClear: () => void;
  /** #help-btn click → toggleHelp (preceded by stopPropagation, per help.js). */
  onHelp: () => void;
  /** #file-upload change → uploadFilesToPane(files); input cleared after read. */
  onFiles: (files: File[]) => void;
}

export function UploadControls({
  onUpload,
  onSelection,
  onClear,
  onHelp,
  onFiles,
}: UploadControlsProps) {
  return (
    <>
      <button
        className="help-btn"
        id="help-btn"
        type="button"
        title="hotkey hints"
        aria-label="hotkey hints"
        onPointerDown={onPointerDownNoBlur}
        onClick={(e) => {
          e.stopPropagation();
          onHelp();
        }}
      >
        ?
      </button>
      <button
        className="upload-btn"
        id="upload-btn"
        type="button"
        title="upload file"
        aria-label="upload file"
        disabled
        onClick={onUpload}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <path d="M17 8 12 3 7 8" />
          <path d="M12 3v12" />
        </svg>
      </button>
      <button
        className="selection-btn"
        id="selection-btn"
        type="button"
        title="selection mode"
        aria-label="selection mode"
        aria-pressed="false"
        disabled
        onPointerDown={onPointerDownNoBlur}
        onClick={onSelection}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M4 4h16" />
          <path d="M9 4v16" />
          <path d="M15 4v16" />
          <path d="M4 20h16" />
        </svg>
      </button>
      <button
        className="clear-pane-btn"
        id="clear-pane-btn"
        type="button"
        title="clear pane"
        aria-label="clear pane"
        disabled
        onPointerDown={onPointerDownNoBlur}
        onClick={onClear}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.1"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="m7 21-4-4 9.5-9.5 4 4L7 21Z" />
          <path d="m15 5 4 4" />
          <path d="M11 21h10" />
        </svg>
      </button>
      <input
        id="file-upload"
        type="file"
        multiple
        hidden
        onChange={(e) => {
          const files = Array.from(e.target.files || []);
          e.target.value = "";
          onFiles(files);
        }}
      />
    </>
  );
}
