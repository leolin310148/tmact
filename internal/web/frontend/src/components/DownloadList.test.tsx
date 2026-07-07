import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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

describe("DownloadList", () => {
  it("renders nothing when closed", () => {
    render(<DownloadList state={null} onClose={() => {}} />);
    expect(document.getElementById("download-overlay")).toBeNull();
  });

  it("shows the scanning state while loading", () => {
    render(<DownloadList state={makeState({ loading: true })} onClose={() => {}} />);
    expect(screen.getByText("掃描 pane 檔案中…")).toBeTruthy();
  });

  it("shows the empty message when the scan finds nothing", () => {
    render(<DownloadList state={makeState()} onClose={() => {}} />);
    expect(screen.getByText("pane 輸出中找不到可下載的檔案")).toBeTruthy();
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
  });

  it("closes on backdrop click and Escape, but not on panel click", () => {
    const onClose = vi.fn();
    render(<DownloadList state={makeState()} onClose={onClose} />);
    fireEvent.click(document.querySelector(".download-card") as HTMLElement);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(document.getElementById("download-overlay") as HTMLElement);
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
