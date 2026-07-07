// downloadScan — extracts file-path-looking tokens from pane output for the
// selection-mode download list. The scan is deliberately liberal (the server's
// /api/files/check stats every candidate and only returns readable regular
// files), but it still pre-filters obvious non-paths so a big pane buffer does
// not turn into hundreds of pointless stat calls.
//
// Scanning runs bottom-up (last line first) so the newest output — the files an
// agent just wrote — leads the download list.

const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;
// Wrapping/punctuation that terminal output typically glues onto a path.
const LEADING_TRIM_RE = /^[('"`<[{]+/;
const TRAILING_TRIM_RE = /[.,:;!?)'"`>\]}]+$/;
// A bare relative token (no ./ prefix) must end in a file extension to count —
// "src/main.go" yes, "feature/branch-name" style noise mostly filtered by stat.
const EXT_RE = /\.[A-Za-z0-9][A-Za-z0-9._-]*$/;
// Tokens are split on whitespace plus the box-drawing borders agents wrap
// tables and banners with, so paths inside │ … │ frames still surface.
const TOKEN_SPLIT_RE = /[\s│┃║]+/;

export const DOWNLOAD_SCAN_LIMIT = 400;

export function scanDownloadablePaths(text: string, limit = DOWNLOAD_SCAN_LIMIT): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const lines = text.split("\n");
  for (let i = lines.length - 1; i >= 0 && out.length < limit; i--) {
    for (const raw of (lines[i] ?? "").split(TOKEN_SPLIT_RE)) {
      const path = candidatePath(raw);
      if (!path || seen.has(path)) continue;
      seen.add(path);
      out.push(path);
      if (out.length >= limit) break;
    }
  }
  return out;
}

function candidatePath(raw: string): string {
  const t = raw.replace(LEADING_TRIM_RE, "").replace(TRAILING_TRIM_RE, "");
  if (!t || t.length > 512 || t.includes("\x00")) return "";
  const scheme = URL_SCHEME_RE.exec(t);
  if (scheme) {
    // Only file:// URLs are downloadable; http(s) etc. are links, not files.
    return scheme[0].toLowerCase() === "file://" ? t : "";
  }
  if (t.startsWith("~")) return ""; // server rejects home-relative paths
  if (t.startsWith("/") || t.startsWith("./") || t.startsWith("../")) return t;
  // Relative "dir/file.ext" tokens: require an extension so prose fractions
  // ("and/or") and branch names don't flood the server check.
  if (t.includes("/") && EXT_RE.test(t)) return t;
  return "";
}

export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "";
  if (bytes < 1024) return bytes + " B";
  const units = ["KB", "MB", "GB", "TB"];
  let v = bytes;
  let u = -1;
  do {
    v /= 1024;
    u++;
  } while (v >= 1024 && u < units.length - 1);
  return (v >= 100 ? Math.round(v) : v.toFixed(1)) + " " + units[u];
}
