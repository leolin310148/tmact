import { describe, expect, it } from "vitest";
import { formatFileSize, scanDownloadablePaths } from "./downloadScan";

describe("scanDownloadablePaths", () => {
  it("collects absolute, cwd-relative, and file URL paths", () => {
    const text = [
      "wrote /tmp/report.txt just now",
      "build output at dist/app.log",
      "also ./notes.md and ../shared/config.yaml",
      "url file:///tmp/dump.bin",
    ].join("\n");
    expect(scanDownloadablePaths(text)).toEqual([
      "file:///tmp/dump.bin",
      "./notes.md",
      "../shared/config.yaml",
      "dist/app.log",
      "/tmp/report.txt",
    ]);
  });

  it("scans bottom-up so the newest paths lead the list", () => {
    const text = "/tmp/old.txt\n/tmp/new.txt";
    expect(scanDownloadablePaths(text)).toEqual(["/tmp/new.txt", "/tmp/old.txt"]);
  });

  it("dedupes repeated paths", () => {
    const text = "/tmp/a.txt\n/tmp/a.txt\n/tmp/a.txt";
    expect(scanDownloadablePaths(text)).toEqual(["/tmp/a.txt"]);
  });

  it("strips wrapping quotes, brackets, and trailing punctuation", () => {
    const text = 'saved "(/tmp/out dir/report.txt)". done';
    // quotes/parens/trailing punctuation trimmed; inner spaces split tokens, so
    // only the extension-bearing tail token survives here.
    expect(scanDownloadablePaths('open `/tmp/report.txt`.')).toEqual(["/tmp/report.txt"]);
    expect(scanDownloadablePaths(text)).toContain("dir/report.txt");
  });

  it("finds paths inside box-drawing table borders", () => {
    const text = "│ /tmp/inside.txt │ 12 KB │";
    expect(scanDownloadablePaths(text)).toEqual(["/tmp/inside.txt"]);
  });

  it("rejects prose, remote URLs, home-relative paths, and bare words", () => {
    const text = [
      "and/or either",
      "https://example.test/report.txt",
      "~/secret.txt",
      "plainword",
      "feature/branch-name",
    ].join("\n");
    expect(scanDownloadablePaths(text)).toEqual([]);
  });

  it("caps the number of candidates", () => {
    const lines: string[] = [];
    for (let i = 0; i < 50; i++) lines.push(`/tmp/file-${i}.txt`);
    expect(scanDownloadablePaths(lines.join("\n"), 10)).toHaveLength(10);
  });
});

describe("formatFileSize", () => {
  it("formats byte counts for the list rows", () => {
    expect(formatFileSize(0)).toBe("0 B");
    expect(formatFileSize(512)).toBe("512 B");
    expect(formatFileSize(2048)).toBe("2.0 KB");
    expect(formatFileSize(5 * 1024 * 1024)).toBe("5.0 MB");
    expect(formatFileSize(-1)).toBe("");
  });
});
