import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
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
  it("uses Taiwan today and Taiwan midnight for the range even before UTC rolls over", async () => {
    vi.setSystemTime(new Date("2026-10-20T16:30:00Z")); // Taiwan Oct 21, 00:30.
    vi.mocked(loadAgentUsageHistory).mockResolvedValue({ res: { ok: true } as Response, data: { timezone: "Asia/Taipei", machines: [{
      machine: "local", updated_at: "2026-10-20T16:15:00Z",
      daily: [{ date: "2026-10-21", provider: "codex", usd: 7 }],
      usage: ["2026-10-14T15:59:59Z", "2026-10-14T16:00:00Z", "2026-10-20T16:15:00Z"].map((at) => ({ at, provider: "codex", windows: [{ name: "weekly", used_percent: 30 }] })),
    }] } });
    render(<UsageTrendDialog onClose={vi.fn()} />);
    await act(async () => {});
    expect(screen.getByText("每日 Cost · $7.00 USD")).toBeInTheDocument();
    const usage = screen.getByRole("img", { name: "共用帳號 codex/weekly 已使用額度趨勢" });
    expect(usage.querySelectorAll("circle")).toHaveLength(2);
    expect(usage.textContent).toContain("2026/10/21");
    expect(screen.getByText(/local · 趨勢更新/)).toHaveTextContent("2026/10/21");
  });
  it("shows shared account usage once while keeping machine cost totals and filters", async () => {
    const now = new Date().toISOString();
    vi.mocked(loadAgentUsageHistory).mockResolvedValue({ res: { ok: true } as Response, data: { timezone: "UTC", machines: ["local", "mini", "nb429", "mbp26"].map((machine, i) => ({
      machine,
      daily: [{ date: now.slice(0, 10), provider: "codex", usd: i + 1 }],
      // Legacy peers may still return duplicate quota histories.
      usage: [{ at: now, provider: "codex", windows: [{ name: "weekly", used_percent: 30 }] }],
    })) } });
    render(<UsageTrendDialog onClose={vi.fn()} />);
    await act(async () => {});
    expect(screen.getByText("每日 Cost · $10.00 USD")).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: /已使用額度趨勢/ })).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Cost 機器"), { target: { value: "mini" } });
    expect(screen.getByText("每日 Cost · $2.00 USD")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "共用帳號 codex/weekly 已使用額度趨勢" })).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: /已使用額度趨勢/ })).toHaveLength(1);
  });
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
