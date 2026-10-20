import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadAgentUsageHistory } from "../api/client";
import UsageTrendDialog from "./UsageTrendDialog";

vi.mock("../api/client", () => ({ loadAgentUsageHistory: vi.fn() }));

beforeEach(() => {
  vi.useFakeTimers();
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.resetAllMocks(); });

describe("background usage trends", () => {
  it("shows preparation rather than zero cost, then polls until ready and cleans up", async () => {
    vi.mocked(loadAgentUsageHistory)
      .mockResolvedValueOnce({ res: { ok: true } as Response, data: { timezone: "UTC", machines: [{ machine: "local", daily: [], usage: [], pending: true }] } })
      .mockResolvedValue({ res: { ok: true } as Response, data: { timezone: "UTC", machines: [{ machine: "local", daily: [], usage: [], updated_at: "2026-10-20T10:00:00Z", cost_updated_at: "2026-10-20T09:00:00Z" }] } });
    const { unmount } = render(<UsageTrendDialog onClose={vi.fn()} />);
    await act(async () => {});
    expect(screen.getByRole("status")).toHaveTextContent("背景正在準備趨勢資料");
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    await act(() => vi.advanceTimersByTimeAsync(5_000));
    expect(screen.getByRole("img", { name: "每日 cost 長條圖" })).toBeInTheDocument();
    expect(screen.getByText(/Cost 更新/)).toBeInTheDocument();
    expect(loadAgentUsageHistory).toHaveBeenCalledTimes(2);
    unmount();
    await act(() => vi.advanceTimersByTimeAsync(60_000));
    expect(loadAgentUsageHistory).toHaveBeenCalledTimes(2);
  });

  it("keeps cached data and its timestamps visible when a peer update fails", async () => {
    vi.mocked(loadAgentUsageHistory).mockResolvedValue({ res: { ok: true } as Response, data: { timezone: "UTC", machines: [{ machine: "mini", daily: [], usage: [], updated_at: "2026-10-20T10:00:00Z", error: "遠端背景更新失敗，保留上次結果" }] } });
    render(<UsageTrendDialog onClose={vi.fn()} />);
    await act(async () => {});
    expect(screen.getByRole("status")).toHaveTextContent("保留上次結果");
    expect(screen.getByText(/mini · 趨勢更新/)).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "每日 cost 長條圖" })).toBeInTheDocument();
  });
});
