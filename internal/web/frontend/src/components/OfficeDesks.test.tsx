import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import { OfficeDesks } from "./OfficeDesks";

// The shared menu content fetches the recently-closed history on open; keep
// these tests offline and history-less.
vi.mock("../api/client", () => ({
  loadClosedSessions: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { sessions: [] } }),
  ),
  killSession: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { ok: true } }),
  ),
  reopenSession: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { ok: true } }),
  ),
  reportHumanActivity: vi.fn(),
}));

afterEach(cleanup);

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

describe("OfficeDesks overflow", () => {
  it("shows peer badges on visible remote desks", () => {
    render(
      <OfficeDesks
        panes={[
          pane({
            target: "peer-a@%2",
            pane_id: "peer-a@%2",
            session: "peer-a@work",
            peer: "peer-a",
            runtime: "codex",
          }),
        ]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const remoteDesk = screen.getByRole("button", { name: "Select pane peer-a work, idle" });
    expect(remoteDesk.querySelector(".desk-peer")).toHaveTextContent("peer-a");
    expect(remoteDesk.querySelector(".desk-label")).toHaveTextContent("work");
  });

  it("shows peer badges for collapsed remote panes", () => {
    render(
      <OfficeDesks
        panes={[
          pane({ target: "local", pane_id: "%1", session: "work" }),
          pane({
            target: "peer-a@%2",
            pane_id: "peer-a@%2",
            session: "peer-a@work",
            peer: "peer-a",
          }),
        ]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /show 2 more panes/i }));

    const remoteRow = screen.getByRole("menuitem", { name: "Select pane peer-a work" });
    expect(remoteRow.querySelector(".peer-badge")).toHaveTextContent("peer-a");
    expect(remoteRow).toHaveAttribute("title", expect.stringContaining("peer-a"));
  });

  it("associates its menu and supports keyboard traversal and activation", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(
      <OfficeDesks
        panes={[
          pane({ target: "a", pane_id: "%1", session: "first" }),
          pane({ target: "b", pane_id: "%2", session: "second" }),
          pane({ target: "c", pane_id: "%3", session: "third" }),
        ]}
        selected={null}
        onSelect={onSelect}
      />,
    );

    const trigger = screen.getByRole("button", { name: /show 3 more panes/i });
    trigger.focus();
    await user.keyboard("{ArrowDown}");

    const menu = screen.getByRole("menu");
    expect(trigger).toHaveAttribute("aria-controls", menu.id);
    expect(menu).toHaveAttribute("aria-labelledby", trigger.id);
    const menuItems = screen.getAllByRole("menuitem");
    expect(menuItems[0]).toHaveFocus();
    await user.keyboard("{End}");
    expect(menuItems[2]).toHaveFocus();
    await user.keyboard("{ArrowUp}{Enter}");

    expect(onSelect).toHaveBeenCalledWith("%2");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("returns focus to the lamp trigger when Escape closes the menu", async () => {
    const user = userEvent.setup();
    render(
      <OfficeDesks
        panes={[pane({ pane_id: "%1", session: "first" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const trigger = screen.getByRole("button", { name: /show 1 more pane/i });
    trigger.focus();
    await user.keyboard(" ");
    expect(screen.getByRole("menuitem")).toHaveFocus();
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });
});
