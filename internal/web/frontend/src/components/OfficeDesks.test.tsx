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

describe("OfficeDesks jump-to-question button", () => {
  const desks = () => [
    pane({ pane_id: "%1", target: "a:0.0", session: "a", runtime: "claude" }),
    pane({ pane_id: "%2", target: "b:0.0", session: "b", runtime: "codex", asking: true }),
    pane({ pane_id: "%3", target: "c:0.0", session: "c", runtime: "claude" }),
    pane({ pane_id: "%4", target: "d:0.0", session: "d", runtime: "claude", asking: true }),
  ];

  it("is hidden when no pane is asking", () => {
    render(
      <OfficeDesks
        panes={[pane({ runtime: "claude" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /asking a question/ })).toBeNull();
  });

  it("is hidden when the only asking pane is already selected", () => {
    render(
      <OfficeDesks
        panes={[pane({ runtime: "claude", asking: true })]}
        selected="%1"
        onSelect={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /asking a question/ })).toBeNull();
  });

  it("ignores stale asking panes", () => {
    render(
      <OfficeDesks
        panes={[pane({ runtime: "claude", asking: true, stale: true })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /asking a question/ })).toBeNull();
  });

  it("scrolls to and selects the next asking pane", async () => {
    const scrolled: string[] = [];
    const orig = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) {
      scrolled.push(this.dataset.paneId ?? "");
    };
    try {
      const onSelect = vi.fn();
      render(<OfficeDesks panes={desks()} selected="%1" onSelect={onSelect} />);
      const btn = screen.getByRole("button", { name: /next of 2 panes asking a question/ });
      expect(btn.textContent).toContain("2");
      await userEvent.click(btn);
      expect(onSelect).toHaveBeenCalledWith("%2");
      expect(scrolled).toEqual(["%2"]);
    } finally {
      HTMLElement.prototype.scrollIntoView = orig;
    }
  });

  it("cycles past the selected asking pane and wraps", async () => {
    const onSelect = vi.fn();
    render(<OfficeDesks panes={desks()} selected="%4" onSelect={onSelect} />);
    // %4 is selected, so only %2 counts and is the wrap-around target.
    await userEvent.click(screen.getByRole("button", { name: "Jump to the pane asking a question" }));
    expect(onSelect).toHaveBeenCalledWith("%2");
  });
});

describe("OfficeDesks order", () => {
  const panes = [
    pane({ target: "a", pane_id: "%1", session: "alpha", runtime: "claude" }),
    pane({ target: "b", pane_id: "%2", session: "bravo", runtime: "codex", running: true }),
    pane({ target: "c", pane_id: "%3", session: "charlie", runtime: "claude", asking: true }),
  ];
  const deskOrder = (container: HTMLElement) =>
    Array.from(container.querySelectorAll<HTMLElement>("[data-pane-id]")).map(
      (el) => el.dataset.paneId,
    );

  it("seats desks by name by default", () => {
    const { container } = render(<OfficeDesks panes={panes} selected={null} onSelect={() => {}} />);
    expect(deskOrder(container)).toEqual(["%1", "%2", "%3"]);
  });

  it("seats asking, then working desks first in activity order", () => {
    const { container } = render(
      <OfficeDesks panes={panes} selected={null} onSelect={() => {}} sortMode="activity" />,
    );
    expect(deskOrder(container)).toEqual(["%3", "%2", "%1"]);
  });
});
