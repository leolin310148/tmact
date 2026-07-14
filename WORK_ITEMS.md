# tmact bug fix and UI/UX work items

This queue is consumed from top to bottom by the managed `tmact loop` in
`.tmact/tmact-work-items-loop.yaml`. Each loop run may complete at most one
unchecked item. The implementation, tests, and checkbox update belong in the
same commit on `main`.

## Work items

- [x] **01 — Prevent stale download-scan results from replacing a newer dialog.** Reproduce the race where `/api/files/check` for pane A finishes after the dialog has been closed or reopened for pane B. Add request identity or cancellation so only the current scan can update state, and cover close/reopen and pane-switch cases with regression tests.

- [x] **02 — Prevent overlapping usage polling requests.** A slow `/api/usage` response must not overlap the next 60-second poll or let an older response replace a newer one. Use a single-flight or completion-scheduled polling strategy, preserve 404 shutdown behavior, and add fake-timer tests for slow, failed, and unmounted requests.

- [x] **03 — Make pane chips fully keyboard operable.** Replace or augment clickable `div` pane chips with correct interactive semantics, Tab focus, Enter/Space activation, selected-state exposure, and visible focus styling without breaking hotkeys, scrolling, or mobile behavior. Extend `Chip` and `StatusLine` tests.

- [ ] **04 — Add complete keyboard behavior to the “more panes” popover.** Give the trigger/list correct relationships and make Arrow keys, Home/End, Enter/Space, Escape, outside click, and focus return work predictably. Preserve pane selection and add focused component tests.

- [ ] **05 — Give direct terminal input an accessible identity and state.** Ensure the invisible direct-input textarea has an appropriate label/description, exposes whether pane input is available, and does not become a confusing empty control to assistive technology when inactive. Add tests for selected, unselected, and selection-mode states.

- [ ] **06 — Improve helper key bar semantics and feedback.** Add accessible names for symbolic keys, `aria-pressed` for sticky Ctrl, `aria-expanded`/`aria-controls` for the overflow toggle, and visible keyboard focus. Keep soft-keyboard no-blur behavior and test both collapsed and armed states.

- [ ] **07 — Make detected prompt choices understandable and keyboard friendly.** Associate the option bar with the detected question text, expose it as a labelled choice group, keep numbered choices readable to screen readers, and support predictable keyboard traversal/activation on mobile and desktop. Add `OptionBar` tests.

- [ ] **08 — Make the quick-input dock an accessible popup.** Synchronize `aria-expanded`, popup ownership, focus entry/return, Escape handling, and empty-state announcement with the dock’s existing imperative open/close state. Preserve touch behavior and add regression tests around open, choose, backdrop close, and pane changes.

- [ ] **09 — Improve quick-button editor labelling and destructive actions.** Give every group and row uniquely associated labels, make remove buttons announce which entry they remove, and keep focus in a sensible row after add/delete. Preserve immediate persistence and add tests for repeated labels and deleting the first/middle/last row.

- [ ] **10 — Complete settings-dialog focus management.** On open, focus a stable control; trap Tab/Shift+Tab inside the modal; on close by button, backdrop, or Escape, restore the invoking control. Avoid stealing focus during async settings loads and add component tests.

- [ ] **11 — Make the download list a robust modal dialog.** Add dialog labelling, focus entry/trap/return, non-interactive loading/error announcements, and keyboard-accessible file rows while preserving native downloads. Test loading, empty, error, populated, Escape, and backdrop states.

- [ ] **12 — Add image-preview loading and failure UX.** Show an explicit loading state, replace broken images with a useful error plus retry action, retain the path/download affordance, and prevent stale load/error events after switching images. Add component tests for load, error, retry, close, and rapid source changes.

- [ ] **13 — Complete markdown-preview dialog semantics and focus behavior.** Add a labelled modal boundary, initial focus, focus trap/return, accessible loading/error status, and safe focus handling while Mermaid rendering finishes. Preserve request cancellation and add regression tests.

- [ ] **14 — Make voice recording and transcription states accessible.** Expose recording, elapsed time, transcribing, confirmation, and error transitions through appropriate dialog/status semantics; move focus to the relevant Stop/Send/Cancel action and restore it when closed. Preserve hotkey recording and add tests around state transitions.

- [ ] **15 — Make the help coachmark overlay usable without a pointer.** Treat it as a labelled dismissible overlay, provide a keyboard-focusable close path, keep focus from escaping behind it, and restore focus to Help when dismissed. Preserve coachmark geometry and add tests for Escape, click, Tab, resize, and empty-tip states.

- [ ] **16 — Preserve each pane’s reading position across pane switches.** Cache scroll position together with pane output, restore it when revisiting a pane, continue following the bottom only for panes that were already bottom-sticky, and keep lazy older-line reveal stable. Add tests for bottom-following and scrolled-up panes.

- [ ] **17 — Surface terminal-stream terminal failure instead of silently appearing healthy.** When retries stop or the selected pane stream is closed unexpectedly, show a distinct, accessible connection state and a safe retry affordance; clear it after reconnect or deselection. Add `usePaneStream` and `ConnStatus` regression tests.

- [ ] **18 — Prevent delayed mobile viewport work from affecting the wrong pane.** Track and cancel the `requestAnimationFrame`/timeout work used to follow the pane bottom when selection, input mode, visibility, or component lifetime changes. Prefer `visualViewport` signals where available and add fake-timer tests for rapid pane switches and unmount.

- [ ] **19 — Establish consistent visible keyboard focus across the web UI.** Audit buttons, links, chips, form fields, floating controls, menus, and overlays; add a coherent `:focus-visible` treatment with sufficient contrast that does not replace useful native focus without an equivalent. Add targeted DOM/CSS assertions and verify representative flows.

- [ ] **20 — Honor reduced-motion preferences across all UI animation.** Audit reconnect indicators, running-agent effects, menus, overlays, coachmarks, recording, and office visuals; under `prefers-reduced-motion: reduce`, remove nonessential motion while retaining state feedback. Add CSS/component coverage and verify both normal and reduced-motion modes.
