import { describe, expect, it } from "vitest";

import { normalizePaneID, paneIDFromURL } from "./paneIntent";

describe("pane notification intents", () => {
  it("accepts raw and URL-encoded tmux pane ids", () => {
    expect(normalizePaneID("%60")).toBe("%60");
    expect(normalizePaneID("%2560")).toBe("%60");
  });

  it("rejects non-pane values", () => {
    expect(normalizePaneID("session:1.0")).toBe("");
    expect(normalizePaneID("https://example.test/?pane=%2560")).toBe("");
    expect(normalizePaneID("%xx")).toBe("");
  });

  it("reads pane from a same-page URL query", () => {
    expect(paneIDFromURL("https://vibe.puni.tw/?pane=%2560")).toBe("%60");
    expect(paneIDFromURL("/?pane=%2561")).toBe("%61");
    expect(paneIDFromURL("/")).toBe("");
  });
});
