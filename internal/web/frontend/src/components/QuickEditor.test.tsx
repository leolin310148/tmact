import { useRef } from "react";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useQuick, type QuickEntry } from "../hooks/useQuick";
import {
  AppStateProvider,
  useAppStateStore,
} from "../store/AppStateContext";
import { QuickEditor } from "./QuickEditor";

const QUICK_KEY = "tmact.quickButtons";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

function EditorBody({ entries }: { entries: QuickEntry[] }) {
  const quick = useQuick({
    wsSend: vi.fn(() => true),
    showInputError: vi.fn(),
    findPane: vi.fn(() => null),
    syncSelectionButton: vi.fn(),
  });
  const seeded = useRef(false);

  if (!seeded.current) {
    quick.quickConfig.common.push(...entries.map((entry) => ({ ...entry })));
    seeded.current = true;
  }

  return <QuickEditor quick={quick} />;
}

function mount(entries: QuickEntry[]) {
  function Harness() {
    const store = useAppStateStore();
    return (
      <AppStateProvider store={store}>
        <EditorBody entries={entries} />
      </AppStateProvider>
    );
  }

  return render(<Harness />);
}

function entry(label: string): QuickEntry {
  return { label, text: `send ${label}` };
}

describe("QuickEditor", () => {
  it("gives groups, repeated-label rows, fields, and remove actions unique names", async () => {
    const user = userEvent.setup();
    mount([entry("Run"), entry("Run")]);

    for (const groupName of [
      "Common · every pane",
      "Claude panes",
      "Codex panes",
      "Shell panes",
    ]) {
      expect(screen.getByRole("group", { name: groupName })).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: `Add button to ${groupName}` }),
      ).toBeInTheDocument();
    }

    const group = screen.getByRole("group", { name: "Common · every pane" });
    const firstRow = within(group).getByRole("group", {
      name: "Common · every pane button 1",
    });
    const secondRow = within(group).getByRole("group", {
      name: "Common · every pane button 2",
    });

    expect(
      within(firstRow).getByRole("textbox", {
        name: "Common · every pane button 1 label",
      }),
    ).toHaveValue("Run");
    expect(
      within(secondRow).getByRole("textbox", {
        name: "Common · every pane button 2 text sent to the pane",
      }),
    ).toHaveValue("send Run");
    expect(
      within(firstRow).getByRole("button", {
        name: 'Remove "Run" from Common · every pane button 1',
      }),
    ).toBeInTheDocument();
    expect(
      within(secondRow).getByRole("button", {
        name: 'Remove "Run" from Common · every pane button 2',
      }),
    ).toBeInTheDocument();

    const firstLabel = within(firstRow).getByRole("textbox", {
      name: "Common · every pane button 1 label",
    });
    await user.clear(firstLabel);
    await user.type(firstLabel, "Deploy");

    expect(
      within(firstRow).getByRole("button", {
        name: 'Remove "Deploy" from Common · every pane button 1',
      }),
    ).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem(QUICK_KEY)!)).toMatchObject({
      common: [{ label: "Deploy", text: "send Run" }, entry("Run")],
    });
  });

  it("focuses and immediately persists a newly added row", async () => {
    const user = userEvent.setup();
    mount([entry("First")]);

    await user.click(
      screen.getByRole("button", { name: "Add button to Common · every pane" }),
    );

    expect(
      screen.getByRole("textbox", {
        name: "Common · every pane button 2 label",
      }),
    ).toHaveFocus();
    expect(JSON.parse(localStorage.getItem(QUICK_KEY)!)).toMatchObject({
      common: [entry("First"), { label: "", text: "" }],
    });
  });

  it.each([
    ["first", 1, "Second", 1],
    ["middle", 2, "Third", 2],
    ["last", 3, "Second", 2],
  ])(
    "keeps focus in the sensible row after deleting the %s row",
    async (_position, deletedNumber, expectedLabel, focusedNumber) => {
      const user = userEvent.setup();
      mount([entry("First"), entry("Second"), entry("Third")]);

      await user.click(
        screen.getByRole("button", {
          name:
            `Remove "${["First", "Second", "Third"][deletedNumber - 1]}" ` +
            `from Common · every pane button ${deletedNumber}`,
        }),
      );

      const focused = screen.getByRole("textbox", {
        name: `Common · every pane button ${focusedNumber} label`,
      });
      expect(focused).toHaveValue(expectedLabel);
      expect(focused).toHaveFocus();
      expect(JSON.parse(localStorage.getItem(QUICK_KEY)!)).toMatchObject({
        common: ["First", "Second", "Third"]
          .filter((_, index) => index !== deletedNumber - 1)
          .map(entry),
      });
    },
  );

  it("moves focus to the group add action after deleting its only row", async () => {
    const user = userEvent.setup();
    mount([entry("Only")]);

    await user.click(
      screen.getByRole("button", {
        name: 'Remove "Only" from Common · every pane button 1',
      }),
    );

    expect(
      screen.getByRole("button", { name: "Add button to Common · every pane" }),
    ).toHaveFocus();
  });
});
