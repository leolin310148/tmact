import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import { MoreChip } from "./MoreChip";

afterEach(cleanup);

function pane(paneID: string, session: string): PaneStatus {
  return {
    target: session + ":0.0",
    pane_id: paneID,
    session,
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
  };
}

const items = [
  { pane: pane("%1", "first"), label: "first" },
  { pane: pane("%2", "second"), label: "second" },
  { pane: pane("%3", "third"), label: "third" },
];

describe("MoreChip keyboard menu", () => {
  it("associates the trigger and menu and exposes rows as menu items", async () => {
    const user = userEvent.setup();
    render(<MoreChip items={items} onSelect={vi.fn()} />);

    const trigger = screen.getByRole("button", { name: "Show 3 more panes" });
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);

    const menu = screen.getByRole("menu");
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(trigger).toHaveAttribute("aria-controls", menu.id);
    expect(menu).toHaveAttribute("aria-labelledby", trigger.id);
    const menuItems = within(menu).getAllByRole("menuitem");
    expect(menuItems).toHaveLength(3);
    for (const item of menuItems) {
      expect(item).toHaveAttribute("tabindex", "-1");
      expect(item).not.toHaveAttribute("aria-pressed");
    }
  });

  it("opens from the keyboard and traverses with arrows and Home/End", async () => {
    const user = userEvent.setup();
    render(<MoreChip items={items} onSelect={vi.fn()} />);

    const trigger = screen.getByRole("button", { name: "Show 3 more panes" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");

    const menuItems = screen.getAllByRole("menuitem");
    expect(menuItems[0]).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(menuItems[1]).toHaveFocus();
    await user.keyboard("{End}");
    expect(menuItems[2]).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(menuItems[0]).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(menuItems[2]).toHaveFocus();
    await user.keyboard("{Home}");
    expect(menuItems[0]).toHaveFocus();
  });

  it("opens on Space, activates with Enter, and returns focus", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<MoreChip items={items} onSelect={onSelect} />);

    const trigger = screen.getByRole("button", { name: "Show 3 more panes" });
    trigger.focus();
    await user.keyboard(" ");
    expect(screen.getAllByRole("menuitem")[0]).toHaveFocus();

    await user.keyboard("{ArrowDown}{Enter}");
    expect(onSelect).toHaveBeenCalledWith("%2");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("closes on Escape or outside pointerdown with predictable focus", async () => {
    const user = userEvent.setup();
    render(
      <>
        <MoreChip items={items} onSelect={vi.fn()} />
        <button type="button">outside</button>
      </>,
    );

    const trigger = screen.getByRole("button", { name: "Show 3 more panes" });
    trigger.focus();
    await user.keyboard("{ArrowUp}");
    expect(screen.getAllByRole("menuitem")[2]).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    const outside = screen.getByRole("button", { name: "outside" });
    outside.focus();
    fireEvent.pointerDown(outside);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(outside).toHaveFocus();
  });

  it("preserves no-blur pointer selection for touch input", () => {
    const onSelect = vi.fn();
    render(
      <>
        <input aria-label="draft" />
        <MoreChip items={items} onSelect={onSelect} />
      </>,
    );

    const draft = screen.getByRole("textbox", { name: "draft" });
    const trigger = screen.getByRole("button", { name: "Show 3 more panes" });
    draft.focus();
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    expect(draft).toHaveFocus();

    const first = screen.getAllByRole("menuitem")[0]!;
    fireEvent.pointerDown(first);
    fireEvent.click(first);
    expect(onSelect).toHaveBeenCalledWith("%1");
    expect(draft).toHaveFocus();
  });
});
