import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import { OverflowMenuContent } from "./OverflowMenu";

const api = vi.hoisted(() => ({
  loadClosedSessions: vi.fn(),
  closePaneWindow: vi.fn(),
  reopenSession: vi.fn(),
  reportHumanActivity: vi.fn(),
}));

vi.mock("../api/client", () => api);

beforeEach(() => {
  vi.clearAllMocks();
  api.loadClosedSessions.mockResolvedValue({
    res: { ok: true } as Response,
    data: { sessions: [] },
  });
  api.closePaneWindow.mockResolvedValue({ res: { ok: true } as Response, data: { ok: true } });
  api.reopenSession.mockResolvedValue({ res: { ok: true } as Response, data: { ok: true } });
});

afterEach(cleanup);

function pane(overrides: Partial<PaneStatus> = {}): PaneStatus {
  return {
    target: "work:0.0",
    pane_id: "%1",
    session: "work",
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

const items = [{ pane: pane(), label: "work" }];

describe("OverflowMenuContent search", () => {
  it("filters closed history too while preserving its explicit expand action", async () => {
    const user = userEvent.setup();
    api.loadClosedSessions.mockResolvedValue({ res: { ok: true } as Response, data: { sessions: [
      { session: "ops-old", peer: "mini", closed_at: "2026-10-20T00:00:00Z" },
      { session: "unrelated", closed_at: "2026-10-20T00:00:00Z" },
    ] } });
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);
    await screen.findByRole("menuitem", { name: "Show 2 recently closed sessions" });
    await user.keyboard("OPS");
    expect(screen.queryByRole("menuitem", { name: "Select pane work" })).not.toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(api.reopenSession).not.toHaveBeenCalled();
    await user.click(screen.getByRole("menuitem", { name: "Show 1 recently closed session" }));
    expect(screen.getByRole("menuitem", { name: "Reopen session ops-old" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Reopen session unrelated" })).not.toBeInTheDocument();
  });
  it("matches peer names and Chinese labels and does not select while composing", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<OverflowMenuContent items={[
      { pane: pane({ peer: "mini", session: "mini@部署", pane_id: "mini@%1" }), label: "部署" },
      { pane: pane({ pane_id: "%2" }), label: "work" },
    ]} onSelect={onSelect} closeRestoring={vi.fn()} />);
    const search = screen.getByRole("searchbox");
    await user.keyboard("MINI");
    expect(screen.getAllByRole("menuitem")).toHaveLength(1);
    fireEvent.change(search, { target: { value: "部署" } });
    fireEvent.keyDown(search, { key: "Enter", isComposing: true, keyCode: 229 });
    expect(onSelect).not.toHaveBeenCalled();
    expect(screen.getByRole("menuitem", { name: "Select pane mini 部署" })).toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith("mini@%1");
  });

  it("keeps editing keys in the search field from arming or closing windows", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);
    await user.keyboard("work{Backspace}{Delete}");
    expect(api.closePaneWindow).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Close window work" })).toHaveTextContent("✕");
  });
});

describe("OverflowMenuContent exit button", () => {
  it("closing ops:1 never targets ops:0 or the whole session", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={[
      { pane: pane({ session: "ops", pane_id: "%10", window_index: 0 }), label: "ops:0" },
      { pane: pane({ session: "ops", pane_id: "%11", window_index: 1 }), label: "ops:1" },
    ]} onSelect={vi.fn()} closeRestoring={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Close window ops:1" }));
    expect(screen.getByRole("button", { name: "Close window ops:0" })).toHaveTextContent("✕");
    await user.click(screen.getByRole("button", { name: "Confirm closing window ops:1" }));
    expect(api.closePaneWindow).toHaveBeenCalledExactlyOnceWith("%11", undefined);
  });
  it("closes only the window on the confirming second press", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    const kill = screen.getByRole("button", { name: "Close window work" });
    expect(kill).toHaveTextContent("✕");

    await user.click(kill);
    expect(api.closePaneWindow).not.toHaveBeenCalled();
    const confirm = screen.getByRole("button", { name: "Confirm closing window work" });
    expect(confirm).toHaveTextContent("close?");

    await user.click(confirm);
    expect(api.closePaneWindow).toHaveBeenCalledWith("%1", undefined);
    expect(api.reportHumanActivity).toHaveBeenCalled();
  });

  it("routes peer sessions to their peer and shows failures inline", async () => {
    api.closePaneWindow.mockResolvedValue({
      res: { ok: false, status: 500 } as Response,
      data: { error: "kill failed" },
    });
    const user = userEvent.setup();
    render(
      <OverflowMenuContent
        items={[
          {
            pane: pane({ pane_id: "z13@%2", session: "z13@remote", peer: "z13" }),
            label: "remote",
          },
        ]}
        onSelect={vi.fn()}
        closeRestoring={vi.fn()}
      />,
    );

    const kill = screen.getByRole("button", { name: "Close window remote" });
    await user.click(kill);
    await user.click(screen.getByRole("button", { name: "Confirm closing window remote" }));

    expect(api.closePaneWindow).toHaveBeenCalledWith("z13@%2", "z13");
    await waitFor(() =>
      expect(screen.getByText("exit remote: kill failed")).toBeInTheDocument(),
    );
  });

  it("arms and closes from the keyboard with Delete", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    const row = screen.getByRole("menuitem", { name: "Select pane work" });
    row.focus();
    await user.keyboard("{Delete}");
    expect(api.closePaneWindow).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Confirm closing window work" }),
    ).toBeInTheDocument();

    await user.keyboard("{Delete}");
    expect(api.closePaneWindow).toHaveBeenCalledWith("%1", undefined);
  });
});

describe("OverflowMenuContent recently closed", () => {
  it("lists closed sessions and reopens one, removing its row", async () => {
    api.loadClosedSessions.mockResolvedValue({
      res: { ok: true } as Response,
      data: {
        sessions: [
          { session: "old", cwd: "/Users/me/w/proj", closed_at: "2026-07-21T00:00:00Z" },
          {
            session: "far",
            cwd: "/srv/app",
            closed_at: "2026-07-20T00:00:00Z",
            peer: "z13",
          },
        ],
      },
    });
    const user = userEvent.setup();
    render(<OverflowMenuContent items={[]} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    const toggle = await screen.findByRole("menuitem", {
      name: "Show 2 recently closed sessions",
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(
      screen.queryByRole("menuitem", { name: "Reopen session old" }),
    ).not.toBeInTheDocument();

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    const reopen = screen.getByRole("menuitem", { name: "Reopen session old" });
    expect(reopen).toHaveTextContent("…/w/proj");
    expect(screen.getByText("recently closed")).toBeInTheDocument();

    await user.click(reopen);
    expect(api.reopenSession).toHaveBeenCalledWith("old", "/Users/me/w/proj", undefined);
    expect(api.reportHumanActivity).toHaveBeenCalled();
    await waitFor(() =>
      expect(
        screen.queryByRole("menuitem", { name: "Reopen session old" }),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("menuitem", { name: "Reopen session far" })).toBeInTheDocument();
  });

  it("shows a reopen failure inline and keeps the row", async () => {
    api.loadClosedSessions.mockResolvedValue({
      res: { ok: true } as Response,
      data: { sessions: [{ session: "old", cwd: "/gone", closed_at: "2026-07-21T00:00:00Z" }] },
    });
    api.reopenSession.mockResolvedValue({
      res: { ok: false, status: 400 } as Response,
      data: { error: "cwd does not exist" },
    });
    const user = userEvent.setup();
    render(<OverflowMenuContent items={[]} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    await user.click(
      await screen.findByRole("menuitem", { name: "Show 1 recently closed session" }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Reopen session old" }));

    await waitFor(() =>
      expect(screen.getByText("reopen old: cwd does not exist")).toBeInTheDocument(),
    );
    expect(screen.getByRole("menuitem", { name: "Reopen session old" })).toBeInTheDocument();
  });

  it("renders the empty state when nothing is hidden or closed", async () => {
    render(<OverflowMenuContent items={[]} onSelect={vi.fn()} closeRestoring={vi.fn()} />);
    await waitFor(() => expect(screen.getByText("no hidden panes")).toBeInTheDocument());
  });
});
