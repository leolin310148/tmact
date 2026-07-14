import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { checkDownloadFiles } from "../api/client";
import { useDownloadList } from "./useDownloadList";
import type { DownloadScanSource } from "./useDownloadList";

vi.mock("../api/client", () => ({ checkDownloadFiles: vi.fn() }));

const mockedCheckDownloadFiles = vi.mocked(checkDownloadFiles);

type CheckResult = Awaited<ReturnType<typeof checkDownloadFiles>>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function okResult(path: string, name: string): CheckResult {
  return {
    res: { ok: true, status: 200 } as Response,
    data: { files: [{ path, name, dir: "/tmp", size: 10 }] },
  };
}

beforeEach(() => {
  mockedCheckDownloadFiles.mockReset();
});

describe("useDownloadList", () => {
  it("keeps the dialog closed when an earlier scan finishes", async () => {
    const pending = deferred<CheckResult>();
    mockedCheckDownloadFiles.mockReturnValueOnce(pending.promise);
    const source: DownloadScanSource = {
      text: "/tmp/a.txt",
      lines: [],
      cwd: "/work/a",
      peer: "peer-a",
    };
    const { result } = renderHook(() => useDownloadList(() => source));

    act(() => result.current.openDownloadList());
    expect(result.current.downloadList).toMatchObject({ loading: true, cwd: "/work/a" });

    act(() => result.current.closeDownloadList());
    await act(async () => pending.resolve(okResult("/tmp/a.txt", "a.txt")));

    expect(result.current.downloadList).toBeNull();
  });

  it("does not let pane A replace a dialog reopened for pane B", async () => {
    const paneA = deferred<CheckResult>();
    const paneB = deferred<CheckResult>();
    mockedCheckDownloadFiles.mockReturnValueOnce(paneA.promise).mockReturnValueOnce(paneB.promise);
    let source: DownloadScanSource = {
      text: "/tmp/a.txt",
      lines: [],
      cwd: "/work/a",
      peer: "peer-a",
    };
    const getSource = () => source;
    const { result } = renderHook(() => useDownloadList(getSource));

    act(() => result.current.openDownloadList());
    source = {
      text: "/tmp/b.txt",
      lines: [],
      cwd: "/work/b",
      peer: "peer-b",
    };
    act(() => result.current.openDownloadList());

    await act(async () => paneB.resolve(okResult("/tmp/b.txt", "b.txt")));
    expect(result.current.downloadList).toMatchObject({
      loading: false,
      cwd: "/work/b",
      peer: "peer-b",
      files: [{ name: "b.txt" }],
    });

    await act(async () => paneA.resolve(okResult("/tmp/a.txt", "a.txt")));
    expect(result.current.downloadList).toMatchObject({
      cwd: "/work/b",
      peer: "peer-b",
      files: [{ name: "b.txt" }],
    });
  });
});
