// Regression coverage for the applySnapshot → renderMode parity gap.
//
// Old app.js applySnapshot called renderMode() on EVERY snapshot (app.js:264),
// not just on selection/focus events. renderMode has imperative side effects —
// the responsive #draft placeholder (mobile vs desktop) and the "Select a pane
// to enable input" #mode-text — that must apply at boot. The React port's
// applySnapshot originally only called bump() (a pure re-render), so on a fresh
// load with no restorable selection (selectPane → renderMode never fires) the
// mobile placeholder showed the desktop ⌘/Ctrl hint and #mode-indicator stayed
// hidden. This test pins that applySnapshot invokes the injected renderMode.

import type { ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  AppStateProvider,
  useAppStateStore,
  type AppStateStore,
} from "../store/AppStateContext";
import type { Snapshot } from "../types/server";
import { useSnapshotStream, type SnapshotStreamDeps } from "./useSnapshotStream";

// useSnapshotStream pulls snapshots from api/client; stub both so applySnapshot
// runs without a network. fetchSnapshot resolves a minimal (empty-pane) snapshot
// so restoreSelection finds no target and renderMode is the side effect under test.
vi.mock("../api/client", () => ({
  fetchSnapshot: vi.fn(async () => ({ ts: "2026-05-30T00:00:00.000Z", panes: {} })),
  subscribeSnapshot: vi.fn(() => () => {}),
}));

afterEach(cleanup);

let mountedStore: AppStateStore | null = null;

function wrapper({ children }: { children: ReactNode }) {
  const store = useAppStateStore();
  mountedStore = store;
  return <AppStateProvider store={store}>{children}</AppStateProvider>;
}

function makeDeps(renderMode: () => void): SnapshotStreamDeps {
  return {
    paneCache: { current: {} as Record<string, string[]> },
    selectPane: vi.fn(),
    clearSelection: vi.fn(),
    syncQuickDock: vi.fn(),
    renderMode,
    closeWS: vi.fn(),
    openWS: vi.fn(),
  };
}

describe("useSnapshotStream applySnapshot", () => {
  it("invokes renderMode() on each applied snapshot (app.js:264 parity)", async () => {
    const renderMode = vi.fn();
    const { result } = renderHook(() => useSnapshotStream(makeDeps(renderMode)), {
      wrapper,
    });

    // Mounting alone must NOT apply a snapshot (App drives the first fetch).
    expect(renderMode).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.refreshSnapshot();
    });
    expect(renderMode).toHaveBeenCalledTimes(1);

    await act(async () => {
      await result.current.refreshSnapshot();
    });
    expect(renderMode).toHaveBeenCalledTimes(2);
  });

  it("uses the daemon-provided stale threshold", () => {
    const { result } = renderHook(() => useSnapshotStream(makeDeps(vi.fn())), {
      wrapper,
    });
    const snapshot = {
      ts: new Date(Date.now() - 11000).toISOString(),
      stale_after_ms: 30000,
    } as Snapshot;
    if (!mountedStore) throw new Error("store was not mounted");
    mountedStore.value.state.snapshot = snapshot;

    expect(result.current.checkStale()).toBe(false);
  });

  it("clears a selected local pane after a snapshot confirms it disappeared", async () => {
    const deps = makeDeps(vi.fn());
    const { result } = renderHook(() => useSnapshotStream(deps), { wrapper });
    if (!mountedStore) throw new Error("store was not mounted");
    mountedStore.value.state.selected = "%45";

    await act(async () => {
      await result.current.refreshSnapshot();
    });

    expect(deps.clearSelection).toHaveBeenCalledWith("%45");
  });

  it("retains a missing peer pane across a transient peer snapshot failure", async () => {
    const deps = makeDeps(vi.fn());
    const { result } = renderHook(() => useSnapshotStream(deps), { wrapper });
    if (!mountedStore) throw new Error("store was not mounted");
    mountedStore.value.state.selected = "mini@%45";

    await act(async () => {
      await result.current.refreshSnapshot();
    });

    expect(deps.clearSelection).not.toHaveBeenCalled();
  });
});
