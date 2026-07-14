import { useCallback, useEffect, useRef, useState } from "react";

import { checkDownloadFiles } from "../api/client";
import { scanDownloadablePaths } from "../lib/downloadScan";
import type { DownloadListState } from "../components/DownloadList";

export interface DownloadScanSource {
  text: string;
  lines: string[];
  cwd: string | null;
  peer: string | null;
}

export function useDownloadList(getSource: () => DownloadScanSource) {
  const [downloadList, setDownloadList] = useState<DownloadListState | null>(null);
  const requestIDRef = useRef(0);

  const closeDownloadList = useCallback(() => {
    requestIDRef.current += 1;
    setDownloadList(null);
  }, []);

  const openDownloadList = useCallback(() => {
    const requestID = ++requestIDRef.current;
    const source = getSource();
    const buffer = source.lines.length ? source.lines.join("\n") : source.text;
    const base: DownloadListState = {
      loading: true,
      error: "",
      files: [],
      cwd: source.cwd,
      peer: source.peer,
    };
    const paths = scanDownloadablePaths(buffer);
    if (!paths.length) {
      setDownloadList({ ...base, loading: false });
      return;
    }

    setDownloadList(base);
    void checkDownloadFiles(paths, source.cwd, source.peer)
      .then(({ res, data }) => {
        if (requestID !== requestIDRef.current) return;
        if (!res.ok) {
          setDownloadList({
            ...base,
            loading: false,
            error: data.error || "掃描失敗(HTTP " + res.status + ")",
          });
          return;
        }
        setDownloadList({
          ...base,
          loading: false,
          files: Array.isArray(data.files) ? data.files : [],
        });
      })
      .catch(() => {
        if (requestID !== requestIDRef.current) return;
        setDownloadList({ ...base, loading: false, error: "掃描失敗 — 連線錯誤" });
      });
  }, [getSource]);

  useEffect(
    () => () => {
      requestIDRef.current += 1;
    },
    [],
  );

  return { downloadList, openDownloadList, closeDownloadList };
}
