import { describe, expect, it } from "vitest";
import { translateDirectKey } from "./keymap";

describe("direct terminal keyboard translation", () => {
  it.each([true, false])("relays Ctrl+C during composition (physical=%s)", (physical) => {
    expect(translateDirectKey(new KeyboardEvent("keydown", {
      key: "Process", code: "KeyC", ctrlKey: true, isComposing: true, keyCode: 229,
    }), physical)).toEqual({ t: "key", k: "C-c" });
  });
  it.each([
    ["KeyA", false, "a"], ["KeyA", true, "A"],
    ["Digit2", true, "@"], ["Slash", true, "?"],
    ["Space", false, " "], ["Enter", true, "\n"],
  ] as const)("maps %s shift=%s to English", (code, shiftKey, s) => {
    expect(translateDirectKey(new KeyboardEvent("keydown", {
      key: "Process", code, shiftKey, isComposing: true, keyCode: 229,
    }), true)).toEqual({ t: "text", s });
  });
  it("preserves mobile composition and browser shortcuts", () => {
    expect(translateDirectKey(new KeyboardEvent("keydown", {
      key: "注", code: "KeyA", isComposing: true,
    }), false)).toBeNull();
    expect(translateDirectKey(new KeyboardEvent("keydown", {
      key: "c", code: "KeyC", metaKey: true,
    }), true)).toBeNull();
  });
  it("preserves navigation during composition", () => {
    expect(translateDirectKey(new KeyboardEvent("keydown", {
      key: "Process", code: "ArrowUp", isComposing: true,
    }), true)).toEqual({ t: "key", k: "Up" });
  });
});
