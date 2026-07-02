import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import { OfficeDesks } from "./OfficeDesks";

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
    expect(remoteRow.querySelector(".desk-more-peer")).toHaveTextContent("peer-a");
    expect(remoteRow).toHaveAttribute("title", expect.stringContaining("peer-a"));
  });
});
