// Split-view slot namespace.
//
// The /split.html shell embeds the app in iframes, each with ?slot=N. Slots
// must persist their pane selection independently, so the localStorage key
// gains a slot suffix. Without the param (mobile / normal single view) the
// key is the original verbatim "tmact.selectedPane" and behavior is
// unchanged. Settings and quick buttons stay shared across slots on purpose.
const slot = (() => {
  try {
    const v = new URLSearchParams(window.location.search).get("slot");
    return v && /^[A-Za-z0-9_-]{1,16}$/.test(v) ? v : "";
  } catch {
    return "";
  }
})();

export const SELECTED_KEY = slot
  ? `tmact.selectedPane.${slot}`
  : "tmact.selectedPane";
