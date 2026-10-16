// Credential-request deep links: a push opens "/?cred=<id>" (or posts
// OPEN_CRED to an open window). Ids are "c_" + lowercase base32, optionally
// prefixed by the owning peer ("mini@c_...").
const CRED_ID = /^(?:[A-Za-z0-9_.-]+@)?c_[a-z2-7]{26}$/;

export function normalizeCredID(raw: unknown): string {
  if (typeof raw !== "string") return "";
  let id = raw.trim();
  if (id.includes("%40")) {
    try {
      id = decodeURIComponent(id);
    } catch {
      return "";
    }
  }
  return CRED_ID.test(id) ? id : "";
}

export function credIDFromURL(rawURL: string): string {
  try {
    const url = new URL(rawURL, window.location.origin);
    return normalizeCredID(url.searchParams.get("cred"));
  } catch {
    return "";
  }
}

export function removeCredParamFromCurrentURL(): void {
  try {
    const url = new URL(window.location.href);
    if (!url.searchParams.has("cred")) return;
    url.searchParams.delete("cred");
    window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
  } catch {
    // Leave the URL untouched when history is unavailable.
  }
}
