import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  AppStateProvider,
  useAppStateStore,
} from "../store/AppStateContext";
import type { Snapshot } from "../types/server";
import { ConnStatus } from "./ConnStatus";

afterEach(() => {
  cleanup();
});

function freshSnapshot(): Snapshot {
  return {
    version: 1,
    ts: new Date().toISOString(),
    generated_by: "test",
    interval_ms: 1000,
    stale_after_ms: 10000,
    summary: {
      sessions: 0,
      panes: 0,
      working: 0,
      asking: 0,
      errors: 0,
    },
    panes: {},
    sessions: {},
  };
}

function mount(text: string, snapshot: Snapshot | null) {
  function Harness() {
    const store = useAppStateStore();
    store.value.state.snapshot = snapshot;
    return (
      <AppStateProvider store={store}>
        <ConnStatus text={text} />
      </AppStateProvider>
    );
  }

  render(<Harness />);
  return document.getElementById("conn-status") as HTMLDivElement;
}

describe("ConnStatus", () => {
  it("hides when the pane stream is open and snapshots are fresh", () => {
    const el = mount("", freshSnapshot());

    expect(el).not.toHaveClass("show");
    expect(el.textContent).toBe("");
  });

  it("shows initial snapshot delivery in the same strip", () => {
    const el = mount("", null);

    expect(el).toHaveClass("show", "conn-status-connecting");
    expect(el.textContent).toBe("status updates connecting...");
  });

  it("shows stale snapshot delivery in the same strip", () => {
    const staleSnapshot = freshSnapshot();
    staleSnapshot.ts = new Date(Date.now() - 11000).toISOString();
    const el = mount("", staleSnapshot);

    expect(el).toHaveClass("show", "conn-status-stale");
    expect(el.textContent).toBe("status updates interrupted - retrying...");
  });

  it("prefers pane stream reconnect text over stale snapshot text", () => {
    const el = mount("pane stream reconnecting...", null);

    expect(el).toHaveClass("show", "conn-status-reconnecting");
    expect(el.textContent).toBe("pane stream reconnecting...");
  });
});
