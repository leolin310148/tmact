import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useUsage } from "../hooks/useUsage";
import UsagePanel from "./UsagePanel";

vi.mock("../hooks/useUsage", () => ({
  useUsage: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe("UsagePanel", () => {
  it("shows a history request failure without displaying an empty chart", async () => {
    vi.mocked(useUsage).mockReturnValue({ disabled: false, snap: { generated_at: "2026-10-20T00:00:00Z", providers: [{ provider: "codex" }] } });
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("暫時離線")));
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
    HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
    render(<UsagePanel />);
    fireEvent.click(screen.getByRole("button", { name: "開啟 Cost / Usage 趨勢" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("暫時離線"));
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
  it("opens trends from the cost panel and closes with Escape", async () => {
    vi.mocked(useUsage).mockReturnValue({ disabled: false, snap: { generated_at: "2026-10-20T00:00:00Z", providers: [{ provider: "codex", spend: { week_usd: 3, month_usd: 5 } }] } });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ machines: [{ machine: "local", daily: [], usage: [] }], timezone: "UTC" }) }));
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
    HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
    render(<UsagePanel />);
    fireEvent.click(screen.getByRole("button", { name: "開啟 Cost / Usage 趨勢" }));
    await waitFor(() => expect(screen.getByText("尚無 usage 歷史紀錄")).toBeInTheDocument());
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
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
