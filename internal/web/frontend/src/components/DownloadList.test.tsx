import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import DownloadList from "./DownloadList";
import type { DownloadListState } from "./DownloadList";

afterEach(cleanup);

function makeState(over: Partial<DownloadListState> = {}): DownloadListState {
  return {
    loading: false,
    error: "",
    files: [],
    cwd: "/work/app",
    peer: "peer-a",
    ...over,
  };
}

function Harness({ downloadState = makeState() }: { downloadState?: DownloadListState }) {
  const [state, setState] = useState<DownloadListState | null>(null);
  return (
    <>
      <button type="button" onClick={() => setState(downloadState)}>
        Open downloads
      </button>
      <DownloadList state={state} onClose={() => setState(null)} />
    </>
  );
}

describe("DownloadList", () => {
  it("renders nothing when closed", () => {
    render(<DownloadList state={null} onClose={() => {}} />);
    expect(document.getElementById("download-overlay")).toBeNull();
  });

  it("shows the scanning state while loading", () => {
    render(<DownloadList state={makeState({ loading: true })} onClose={() => {}} />);
    expect(screen.getByRole("dialog", { name: "下載檔案" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.getByRole("status")).toHaveTextContent("掃描 pane 檔案中…");
  });

  it("shows the empty message when the scan finds nothing", () => {
    render(<DownloadList state={makeState()} onClose={() => {}} />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "pane 輸出中找不到可下載的檔案",
    );
  });

  it("announces scan errors without adding an interactive dead end", () => {
    render(
      <DownloadList
        state={makeState({ error: "掃描失敗 — 連線錯誤" })}
        onClose={() => {}}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("掃描失敗 — 連線錯誤");
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });

  it("renders one peer-aware /api/file link per confirmed file", () => {
    const state = makeState({
      files: [
        { path: "dist/report.txt", name: "report.txt", dir: "/work/app/dist", size: 2048 },
        { path: "/tmp/dump.bin", name: "dump.bin", dir: "/tmp", size: 5 },
      ],
    });
    render(<DownloadList state={state} onClose={() => {}} />);
    const links = Array.from(document.querySelectorAll("a.download-item")) as HTMLAnchorElement[];
    expect(links).toHaveLength(2);
    const first = links[0]!;
    expect(first.getAttribute("href")).toBe(
      "/api/file?path=dist%2Freport.txt&cwd=%2Fwork%2Fapp&peer=peer-a",
    );
    expect(first.textContent).toContain("report.txt");
    expect(first.textContent).toContain("2.0 KB");
    expect(first.tagName).toBe("A");
  });

  it("focuses the modal and traps Tab across native file links", async () => {
    const user = userEvent.setup();
    const state = makeState({
      files: [
        { path: "first.txt", name: "first.txt", dir: "/work/app", size: 1 },
        { path: "last.txt", name: "last.txt", dir: "/work/app", size: 2 },
      ],
    });
    render(<Harness downloadState={state} />);

    await user.click(screen.getByRole("button", { name: "Open downloads" }));

    const close = screen.getByRole("button", { name: "close download list" });
    const links = screen.getAllByRole("link");
    expect(close).toHaveFocus();
    await user.tab({ shift: true });
    expect(links[1]).toHaveFocus();
    await user.tab();
    expect(close).toHaveFocus();
  });

  it("restores focus after close button, backdrop, and Escape closes", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const invoker = screen.getByRole("button", { name: "Open downloads" });

    await user.click(invoker);
    await user.click(screen.getByRole("button", { name: "close download list" }));
    expect(screen.queryByRole("dialog", { name: "下載檔案" })).not.toBeInTheDocument();
    expect(invoker).toHaveFocus();

    await user.click(invoker);
    fireEvent.click(document.querySelector(".download-card") as HTMLElement);
    expect(screen.getByRole("dialog", { name: "下載檔案" })).toBeInTheDocument();
    fireEvent.click(document.getElementById("download-overlay") as HTMLElement);
    expect(screen.queryByRole("dialog", { name: "下載檔案" })).not.toBeInTheDocument();
    expect(invoker).toHaveFocus();

    await user.click(invoker);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "下載檔案" })).not.toBeInTheDocument();
    expect(invoker).toHaveFocus();
  });
});
