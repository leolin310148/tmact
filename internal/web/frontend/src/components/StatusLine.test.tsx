import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { AppStateProvider, useAppStateStore } from "../store/AppStateContext";
import type { PaneStatus, Snapshot } from "../types/server";
import { shouldPinPane, splitPaneItems, StatusLine } from "./StatusLine";

beforeAll(() => {
  // jsdom has no layout; the selected Chip calls scrollIntoView on mount.
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  cleanup();
});

function pane(overrides: Partial<PaneStatus> = {}): PaneStatus {
  return {
    target: "s:0.0",
    pane_id: "%1",
    session: "sess",
    window_index: 0,
    pane_index: 0,
    runtime: "",
    tag: "",
    state: "idle",
    idle: true,
    input_ready: true,
    running: false,
    asking: false,
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function item(p: PaneStatus) {
  return { pane: p, label: p.session };
}

// ---- pure partition logic -------------------------------------------------

describe("shouldPinPane", () => {
  it("pins panes running an agent runtime", () => {
    expect(shouldPinPane(pane({ runtime: "claude" }), null)).toBe(true);
    expect(shouldPinPane(pane({ runtime: "codex" }), null)).toBe(true);
  });

  it("pins the selected pane even with no agent", () => {
    expect(shouldPinPane(pane({ pane_id: "%7", runtime: "" }), "%7")).toBe(true);
  });

  it("pins panes needing attention (asking / prompt)", () => {
    expect(shouldPinPane(pane({ asking: true }), null)).toBe(true);
    expect(
      shouldPinPane(pane({ prompt: { choices: [{ number: 1 }] } }), null),
    ).toBe(true);
  });

  it("collapses idle agent-less panes", () => {
    expect(shouldPinPane(pane(), null)).toBe(false);
    expect(shouldPinPane(pane({ pane_id: "%1" }), "%2")).toBe(false);
  });
});

describe("splitPaneItems", () => {
  it("partitions while preserving order within each group", () => {
    const items = [
      item(pane({ pane_id: "%1", runtime: "claude" })),
      item(pane({ pane_id: "%2" })),
      item(pane({ pane_id: "%3", asking: true })),
      item(pane({ pane_id: "%4" })),
    ];
    const { visible, overflow } = splitPaneItems(items, "%4");
    expect(visible.map((v) => v.pane.pane_id)).toEqual(["%1", "%3", "%4"]);
    expect(overflow.map((v) => v.pane.pane_id)).toEqual(["%2"]);
  });
});

// ---- StatusLine render ----------------------------------------------------

function mount(panes: PaneStatus[], selected: string | null, selectPane = vi.fn()) {
  const snapshot: Snapshot = {
    version: 1,
    ts: "2026-01-01T00:00:00Z",
    generated_by: "test",
    interval_ms: 1000,
    stale_after_ms: 10000,
    summary: { sessions: 0, panes: panes.length, working: 0, asking: 0, errors: 0 },
    sessions: {},
    panes: Object.fromEntries(panes.map((p) => [p.target, p])),
  };
  let state: { paneOrder: string[] } | null = null;
  function Harness() {
    const store = useAppStateStore();
    store.value.state.snapshot = snapshot;
    store.value.state.selected = selected;
    store.value.callbacks = { ...store.value.callbacks, selectPane };
    state = store.value.state;
    return (
      <AppStateProvider store={store}>
        <StatusLine />
      </AppStateProvider>
    );
  }
  const utils = render(<Harness />);
  // StatusLine reassigns state.paneOrder during render; read it back afterwards.
  return { ...utils, getOrder: () => state!.paneOrder };
}

describe("StatusLine overflow", () => {
  it("renders inline panes as buttons with the current selection exposed", () => {
    mount(
      [
        pane({
          target: "a",
          pane_id: "%1",
          session: "first",
          runtime: "claude",
        }),
        pane({
          target: "b",
          pane_id: "%2",
          session: "second",
          runtime: "codex",
        }),
      ],
      "%2",
    );

    expect(screen.getByRole("button", { name: /first/ })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(screen.getByRole("button", { name: /second/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("selects an inline pane from the keyboard", async () => {
    const selectPane = vi.fn();
    const user = userEvent.setup();
    mount(
      [pane({ target: "a", pane_id: "%1", session: "work", runtime: "claude" })],
      null,
      selectPane,
    );

    const chip = screen.getByRole("button", { name: /work/ });
    chip.focus();
    await user.keyboard("{Enter}");
    expect(selectPane).toHaveBeenCalledWith("%1");
  });

  it("shows agent/asking/selected panes inline and collapses the rest", () => {
    const { container } = mount(
      [
        pane({ target: "a", pane_id: "%1", session: "agent", runtime: "claude" }),
        pane({ target: "b", pane_id: "%2", session: "idle-1" }),
        pane({ target: "c", pane_id: "%3", session: "idle-2" }),
        pane({ target: "d", pane_id: "%4", session: "sel" }),
      ],
      "%4",
    );

    const chips = container.querySelector("#chips")!;
    // Two inline panes (agent + selected) + the "more" chip.
    expect(chips.querySelectorAll(".chip:not(.more-chip)").length).toBe(2);
    const more = chips.querySelector(".more-chip")!;
    expect(more).not.toBeNull();
    expect(more.querySelector(".more-count")!.textContent).toBe("2");
    // Popover starts closed.
    expect(container.querySelector(".chip-overflow-pop")).toBeNull();
  });

  it("keeps the more chip without a count when nothing collapses", () => {
    // The chip stays as the entry point to the recently-closed history even
    // with no hidden panes — it just drops the overflow count badge.
    const { container } = mount(
      [pane({ target: "a", pane_id: "%1", runtime: "claude" })],
      "%1",
    );
    const more = container.querySelector(".more-chip");
    expect(more).not.toBeNull();
    expect(more!.querySelector(".more-count")).toBeNull();
  });

  it("freezes paneOrder to the inline chips only", () => {
    const { getOrder } = mount(
      [
        pane({ target: "a", pane_id: "%1", runtime: "claude" }),
        pane({ target: "b", pane_id: "%2", session: "idle" }),
      ],
      null,
    );
    expect(getOrder()).toEqual(["%1"]);
  });

  it("opens the popover on click and selects a collapsed pane", () => {
    const selectPane = vi.fn();
    const { container } = mount(
      [
        pane({ target: "a", pane_id: "%1", runtime: "claude" }),
        pane({ target: "b", pane_id: "%2", session: "hidden" }),
      ],
      "%1",
      selectPane,
    );

    fireEvent.click(container.querySelector(".more-chip")!);
    const pop = container.querySelector(".chip-overflow-pop")!;
    expect(pop).not.toBeNull();

    const row = within(pop as HTMLElement).getByText("hidden");
    fireEvent.click(row);
    expect(selectPane).toHaveBeenCalledWith("%2");
    // Popover closes after a selection.
    expect(container.querySelector(".chip-overflow-pop")).toBeNull();
  });

  it("closes the popover on Escape", () => {
    const { container } = mount(
      [
        pane({ target: "a", pane_id: "%1", runtime: "claude" }),
        pane({ target: "b", pane_id: "%2", session: "hidden" }),
      ],
      "%1",
    );
    fireEvent.click(container.querySelector(".more-chip")!);
    expect(container.querySelector(".chip-overflow-pop")).not.toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(container.querySelector(".chip-overflow-pop")).toBeNull();
  });
});
