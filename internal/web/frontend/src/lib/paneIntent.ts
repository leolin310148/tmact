export function normalizePaneID(rawPaneID: unknown): string {
  if (typeof rawPaneID !== "string") return "";
  const paneID = rawPaneID.trim();
  if (paneID.startsWith("%25")) {
    try {
      const decoded = decodeURIComponent(paneID);
      if (/^%[0-9]+$/.test(decoded)) return decoded;
    } catch {
      return "";
    }
  }
  if (/^%[0-9]+$/.test(paneID)) return paneID;
  try {
    const decoded = decodeURIComponent(paneID);
    if (/^%[0-9]+$/.test(decoded)) return decoded;
  } catch {
    // Ignore malformed percent-encoding.
  }
  return "";
}

export function paneIDFromURL(rawURL: string): string {
  try {
    const url = new URL(rawURL, window.location.origin);
    return normalizePaneID(url.searchParams.get("pane"));
  } catch {
    return "";
  }
}

export function removePaneParamFromCurrentURL(): void {
  try {
    const url = new URL(window.location.href);
    if (!url.searchParams.has("pane")) return;
    url.searchParams.delete("pane");
    window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
  } catch {
    // Leave the URL untouched when history is unavailable.
  }
}
