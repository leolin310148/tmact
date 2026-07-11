export function normalizePaneID(rawPaneID: unknown): string {
  if (typeof rawPaneID !== "string") return "";
  let paneID = rawPaneID.trim();
  if (paneID.includes("%25")) {
    try {
      paneID = decodeURIComponent(paneID);
    } catch {
      return "";
    }
  }
  return /^(?:[A-Za-z0-9_.-]+@)?%[0-9]+$/.test(paneID) ? paneID : "";
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
