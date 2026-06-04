import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
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
