import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import CopyLineBar, {
  buildFileDownloadHref,
  displayColumns,
  selectedDownloadPath,
  smartJoinCommand,
} from "./CopyLineBar";

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

describe("displayColumns", () => {
  it("counts ASCII as one column and CJK as two", () => {
    expect(displayColumns("make build")).toBe(10);
    expect(displayColumns("執行測試")).toBe(8);
    expect(displayColumns("ls 目錄")).toBe(7);
  });
});

describe("smartJoinCommand", () => {
  it("glues a newline after a row exactly as wide as the pane", () => {
    const full = "echo aaaab\nbb";
    expect(smartJoinCommand(full, 0, full.length, 10)).toBe("echo aaaabbb");
  });

  it("keeps real newlines so multi-line scripts run as written", () => {
    const full = "cd /tmp\nmake build";
    expect(smartJoinCommand(full, 0, full.length, 80)).toBe("cd /tmp\nmake build");
  });

  it("keeps trailing-backslash continuations verbatim", () => {
    const full = "docker run \\\n  --rm img";
    expect(smartJoinCommand(full, 0, full.length, 80)).toBe("docker run \\\n  --rm img");
  });

  it("measures the whole grid row even when the selection starts mid-row", () => {
    // Row 0 is "$ echo aaaab" (12 cols == pane width); selecting from the
    // command start (offset 2) must still see the full-width row and glue.
    const full = "$ echo aaaab\nbb\ndone";
    expect(smartJoinCommand(full, 2, full.length, 12)).toBe("echo aaaabbb\ndone");
  });

  it("counts CJK rows at their display width", () => {
    // "echo 測試字" = 5 ASCII + 3 CJK × 2 = 11 columns; pane width 11 → soft wrap.
    const full = "echo 測試字\nsuffix";
    expect(smartJoinCommand(full, 0, full.length, 11)).toBe("echo 測試字suffix");
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

  it("keeps real newlines when the pane width is known", async () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    pre.textContent = "cd /tmp\nmake build";
    document.body.appendChild(pre);
    const startBackground = vi.fn(async () => ({
      res: { ok: true, status: 202 } as Response,
      data: {
        job: {
          id: "job-2",
          status: "finished" as const,
          command: "cd /tmp\nmake build",
          output: "ok",
          exit_code: 0,
          started_at: new Date().toISOString(),
        },
      },
    }));

    render(
      <CopyLineBar paneID="%7" getPaneWidth={() => 80} startBackground={startBackground} />,
    );

    const text = pre.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.textContent?.length ?? 0);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));

    fireEvent.click(document.getElementById("copyline-run") as HTMLButtonElement);

    await waitFor(() =>
      expect(startBackground).toHaveBeenCalledWith("%7", "cd /tmp\nmake build"),
    );
  });

  it("glues terminal soft-wraps when the pane width is known", async () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    // First row is exactly 10 columns (the pane width) → soft wrap.
    pre.textContent = "echo aaaab\nbb";
    document.body.appendChild(pre);
    const startBackground = vi.fn(async () => ({
      res: { ok: true, status: 202 } as Response,
      data: {
        job: {
          id: "job-3",
          status: "finished" as const,
          command: "echo aaaabbb",
          output: "aaaabbb",
          exit_code: 0,
          started_at: new Date().toISOString(),
        },
      },
    }));

    render(
      <CopyLineBar paneID="%7" getPaneWidth={() => 10} startBackground={startBackground} />,
    );

    const text = pre.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.textContent?.length ?? 0);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));

    fireEvent.click(document.getElementById("copyline-run") as HTMLButtonElement);

    await waitFor(() => expect(startBackground).toHaveBeenCalledWith("%7", "echo aaaabbb"));
  });

  it("offers the legacy glue join as a rescue from the arrow menu", async () => {
    const pre = document.createElement("pre");
    pre.id = "content";
    // Neither row is pane-width wide (an agent TUI's own wrap): smart join
    // keeps the newline, the rescue menu item glues it away.
    pre.textContent = "npm run te\n  st";
    document.body.appendChild(pre);
    const startBackground = vi.fn(async () => ({
      res: { ok: true, status: 202 } as Response,
      data: {
        job: {
          id: "job-4",
          status: "finished" as const,
          command: "npm run test",
          output: "passed",
          exit_code: 0,
          started_at: new Date().toISOString(),
        },
      },
    }));

    render(
      <CopyLineBar paneID="%7" getPaneWidth={() => 80} startBackground={startBackground} />,
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
    fireEvent.click(screen.getByRole("menuitem", { name: "接成一行執行" }));

    await waitFor(() => expect(startBackground).toHaveBeenCalledWith("%7", "npm run test"));
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
