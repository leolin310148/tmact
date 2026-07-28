import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import CopyLineBar, { buildFileDownloadHref, selectedDownloadPath } from "./CopyLineBar";

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
});

describe("selectedDownloadPath", () => {
  it("accepts absolute paths, cwd-relative paths, and quoted paths", () => {
    expect(selectedDownloadPath("/tmp/report.txt")).toBe("/tmp/report.txt");
    expect(selectedDownloadPath("dist/report.txt")).toBe("dist/report.txt");
    expect(selectedDownloadPath("./report.txt")).toBe("./report.txt");
    expect(selectedDownloadPath('"build output/report.txt"')).toBe("build output/report.txt");
  });

  it("rejects prose and remote URLs", () => {
    expect(selectedDownloadPath("not a path")).toBe("");
    expect(selectedDownloadPath("https://example.test/report.txt")).toBe("");
  });

  it("joins terminal-wrapped paths before testing", () => {
    expect(selectedDownloadPath("dist/\n  report.txt")).toBe("dist/report.txt");
  });
});

describe("buildFileDownloadHref", () => {
  it("builds a peer-aware file download URL", () => {
    expect(buildFileDownloadHref("dist/report.txt", "/work/app", "peer-a")).toBe(
      "/api/file?path=dist%2Freport.txt&cwd=%2Fwork%2Fapp&peer=peer-a",
    );
  });
});

describe("CopyLineBar download action", () => {
  it("shows a download link when the pane selection is a path", () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    pre.textContent = "dist/report.txt";
    document.body.appendChild(pre);

    render(<CopyLineBar cwd="/work/app" peer="peer-a" />);

    const text = pre.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.textContent?.length ?? 0);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));

    const link = document.getElementById("copyline-download") as HTMLAnchorElement | null;
    expect(link).not.toBeNull();
    expect(link?.hidden).toBe(false);
    expect(link?.getAttribute("href")).toBe(
      "/api/file?path=dist%2Freport.txt&cwd=%2Fwork%2Fapp&peer=peer-a",
    );
  });
});

describe("CopyLineBar run command action", () => {
  it("joins terminal wraps and runs the command as a background job", async () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    pre.textContent = "npm run te\n  st";
    document.body.appendChild(pre);
    const startBackground = vi.fn(async () => ({
      res: { ok: true, status: 202 } as Response,
      data: {
        job: {
          id: "job-1",
          status: "finished" as const,
          command: "npm run test",
          output: "passed",
          exit_code: 0,
          started_at: new Date().toISOString(),
        },
      },
    }));

    render(<CopyLineBar paneID="%7" startBackground={startBackground} />);

    const text = pre.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.textContent?.length ?? 0);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));

    fireEvent.click(document.getElementById("copyline-run") as HTMLButtonElement);

    await waitFor(() => expect(startBackground).toHaveBeenCalledWith("%7", "npm run test"));
    expect(screen.getByRole("dialog", { name: "Command output" })).toBeInTheDocument();
    expect(screen.getByText("passed")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "close command output" }));
    expect(screen.queryByRole("dialog", { name: "Command output" })).not.toBeInTheDocument();
  });

  it("runs the command in a new tmux session and selects its pane from the arrow menu", async () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    pre.textContent = "make test";
    document.body.appendChild(pre);
    const startTmux = vi.fn(async () => ({
      res: { ok: true, status: 201 } as Response,
      data: { pane_id: "%12", session: "work-run" },
    }));
    const onSelectPane = vi.fn();

    render(
      <CopyLineBar
        paneID="mini@%7"
        peer="mini"
        startTmux={startTmux}
        waitForPane={async () => {}}
        onSelectPane={onSelectPane}
      />,
    );

    const text = pre.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.textContent?.length ?? 0);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));

    fireEvent.click(document.getElementById("copyline-run-arrow") as HTMLButtonElement);
    fireEvent.click(screen.getByRole("menuitem", { name: "Run in tmux session" }));

    await waitFor(() => expect(startTmux).toHaveBeenCalledWith("mini@%7", "make test"));
    expect(onSelectPane).toHaveBeenCalledWith("mini@%12");
  });
});
