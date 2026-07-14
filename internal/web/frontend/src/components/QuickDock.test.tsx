import {
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useQuick, type QuickEntry } from "../hooks/useQuick";
import {
  AppStateProvider,
  useAppState,
  useAppStateStore,
} from "../store/AppStateContext";
import type { InputMsg, PaneStatus } from "../types/server";
import { QuickDock } from "./QuickDock";

afterEach(cleanup);

interface HarnessProps {
  selected: string | null;
  entries: QuickEntry[];
  wsSend: (message: InputMsg) => boolean;
}

function DockBody({ selected, entries, wsSend }: HarnessProps) {
  const { state } = useAppState();
  state.selected = selected;
  const quick = useQuick({
    wsSend,
    showInputError: vi.fn(),
    findPane: (id) => (id ? ({ runtime: "shell" } as PaneStatus) : null),
    syncSelectionButton: vi.fn(),
  });
  const seededRef = useRef(false);
  const previousSelectionRef = useRef(selected);

  if (!seededRef.current) {
    quick.quickConfig.common.push(...entries);
    seededRef.current = true;
  }

  useEffect(() => {
    quick.wireQuick();
  }, [quick.wireQuick]);

  useLayoutEffect(() => {
    if (previousSelectionRef.current !== selected) {
      quick.closeQuickMenu(selected !== null);
      previousSelectionRef.current = selected;
    }
    quick.syncQuickDock();
  }, [quick, selected]);

  return <QuickDock quick={quick} />;
}

function Harness(props: HarnessProps) {
  const store = useAppStateStore();
  return (
    <AppStateProvider store={store}>
      <DockBody {...props} />
    </AppStateProvider>
  );
}

function mount({
  selected = "%1",
  entries = [
    { label: "Status", text: "status?" },
    { label: "Compact", text: "/compact" },
  ],
  wsSend = vi.fn(() => true),
}: Partial<HarnessProps> = {}) {
  const props = { selected, entries, wsSend };
  return { ...render(<Harness {...props} />), props };
}

describe("QuickDock", () => {
  it("owns a labelled popup, synchronizes expanded state, and focuses its first action", async () => {
    const user = userEvent.setup();
    mount();

    const trigger = screen.getByRole("button", { name: "quick input" });
    const popup = document.getElementById("qb-menu")!;
    expect(trigger).toHaveAttribute("aria-haspopup", "dialog");
    expect(trigger).toHaveAttribute("aria-controls", "qb-menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(popup).toHaveAttribute("aria-hidden", "true");

    await user.click(trigger);

    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("dialog", { name: "quick input" })).toBe(popup);
    expect(popup).toHaveAttribute("aria-hidden", "false");
    expect(screen.getByRole("button", { name: "Status" })).toHaveFocus();
  });

  it("chooses an action, closes the popup, and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    const wsSend = vi.fn(() => true);
    mount({ wsSend });
    const trigger = screen.getByRole("button", { name: "quick input" });

    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: "Compact" }));

    expect(wsSend).toHaveBeenCalledWith({ t: "send", s: "/compact" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
  });

  it("closes from the backdrop or Escape and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    mount();
    const trigger = screen.getByRole("button", { name: "quick input" });
    const backdrop = document.getElementById("qb-backdrop")!;

    await user.click(trigger);
    fireEvent.click(backdrop);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    await user.keyboard("{Escape}");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
  });

  it("announces and focuses the empty state when opened", async () => {
    const user = userEvent.setup();
    mount({ entries: [] });

    await user.click(screen.getByRole("button", { name: "quick input" }));

    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("No quick buttons for this pane");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveAttribute("aria-atomic", "true");
    expect(status).toHaveFocus();
  });

  it("closes on pane changes and does not leave focus in a hidden popup", async () => {
    const user = userEvent.setup();
    const { props, rerender } = mount();
    const trigger = screen.getByRole("button", { name: "quick input" });

    await user.click(trigger);
    rerender(<Harness {...props} selected="%2" />);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    rerender(<Harness {...props} selected={null} />);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(document.getElementById("qb-dock")).not.toContainElement(
      document.activeElement as HTMLElement | null,
    );
  });
});
