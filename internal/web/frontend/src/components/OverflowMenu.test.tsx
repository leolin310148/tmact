import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import { OverflowMenuContent } from "./OverflowMenu";

const api = vi.hoisted(() => ({
  loadClosedSessions: vi.fn(),
  killSession: vi.fn(),
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
  api.killSession.mockResolvedValue({ res: { ok: true } as Response, data: { ok: true } });
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

describe("OverflowMenuContent exit button", () => {
  it("kills the session only on the confirming second press", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    const kill = screen.getByRole("button", { name: "Close session work" });
    expect(kill).toHaveTextContent("✕");

    await user.click(kill);
    expect(api.killSession).not.toHaveBeenCalled();
    const confirm = screen.getByRole("button", { name: "Confirm closing session work" });
    expect(confirm).toHaveTextContent("kill?");

    await user.click(confirm);
    expect(api.killSession).toHaveBeenCalledWith("work", undefined);
    expect(api.reportHumanActivity).toHaveBeenCalled();
  });

  it("routes peer sessions to their peer and shows failures inline", async () => {
    api.killSession.mockResolvedValue({
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

    const kill = screen.getByRole("button", { name: "Close session remote" });
    await user.click(kill);
    await user.click(screen.getByRole("button", { name: "Confirm closing session remote" }));

    expect(api.killSession).toHaveBeenCalledWith("remote", "z13");
    await waitFor(() =>
      expect(screen.getByText("exit remote: kill failed")).toBeInTheDocument(),
    );
  });

  it("arms and kills from the keyboard with Delete", async () => {
    const user = userEvent.setup();
    render(<OverflowMenuContent items={items} onSelect={vi.fn()} closeRestoring={vi.fn()} />);

    const row = screen.getByRole("menuitem", { name: "Select pane work" });
    row.focus();
    await user.keyboard("{Delete}");
    expect(api.killSession).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Confirm closing session work" }),
    ).toBeInTheDocument();

    await user.keyboard("{Delete}");
    expect(api.killSession).toHaveBeenCalledWith("work", undefined);
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

    const reopen = await screen.findByRole("menuitem", { name: "Reopen session old" });
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

    await user.click(await screen.findByRole("menuitem", { name: "Reopen session old" }));

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
