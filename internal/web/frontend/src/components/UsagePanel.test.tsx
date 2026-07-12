import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useUsage } from "../hooks/useUsage";
import UsagePanel from "./UsagePanel";

vi.mock("../hooks/useUsage", () => ({
  useUsage: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("UsagePanel", () => {
  it("renders a weekly-only Codex window without an empty session row", () => {
    vi.mocked(useUsage).mockReturnValue({
      disabled: false,
      snap: {
        generated_at: "2026-07-13T00:00:00Z",
        providers: [
          {
            provider: "codex",
            plan: "prolite",
            windows: [
              {
                name: "weekly",
                used_percent: 2,
                window_minutes: 10080,
                resets_at: "2099-07-20T00:00:00Z",
              },
            ],
          },
        ],
      },
    });

    const { container } = render(<UsagePanel />);

    expect(screen.getByText("cx")).toBeInTheDocument();
    expect(screen.getByText("7d")).toBeInTheDocument();
    expect(screen.getByText("98%")).toBeInTheDocument();
    expect(container.querySelectorAll(".u-remain")).toHaveLength(1);
  });

  it("labels the normal session and weekly rows by duration", () => {
    vi.mocked(useUsage).mockReturnValue({
      disabled: false,
      snap: {
        generated_at: "2026-07-13T00:00:00Z",
        providers: [
          {
            provider: "codex",
            windows: [
              { name: "session", used_percent: 10, window_minutes: 300 },
              { name: "weekly", used_percent: 20, window_minutes: 10080 },
            ],
          },
        ],
      },
    });

    render(<UsagePanel />);

    expect(screen.getByText("5h")).toBeInTheDocument();
    expect(screen.getByText("7d")).toBeInTheDocument();
  });
});
