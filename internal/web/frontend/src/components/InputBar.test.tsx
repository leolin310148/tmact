// Regression coverage for the same class of bug as UploadControls: #record-btn
// and #send-btn are enabled imperatively (syncRecordButton / selectPane write
// `el.disabled = false`), so they must NOT render a static React `disabled`
// prop — that would make React's synthetic event system suppress their onClick
// forever (shouldPreventMouseEvent reads the fiber props, never the live DOM),
// even after App enables them on the DOM. See InputBar's PARITY MODEL comment.

import { createRef } from "react";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import InputBar from "./InputBar";

afterEach(cleanup);

function renderInputBar() {
  const onRecord = vi.fn();
  const onSend = vi.fn();
  render(
    <InputBar
      keyBar={null}
      modeIndicator={null}
      draft={null}
      direct={false}
      recordBtnRef={createRef<HTMLButtonElement>()}
      sendBtnRef={createRef<HTMLButtonElement>()}
      onRecord={onRecord}
      onSend={onSend}
    />,
  );
  return { onRecord, onSend };
}

const BUTTONS = [
  ["record-btn", "onRecord"],
  ["send-btn", "onSend"],
] as const;

describe("InputBar record/send buttons", () => {
  it.each(BUTTONS)(
    "#%s fires %s after App enables it on the DOM (no static `disabled` prop)",
    (id, handlerName) => {
      const handlers = renderInputBar();
      const btn = document.getElementById(id) as HTMLButtonElement | null;
      expect(btn).not.toBeNull();
      // Mirror App's imperative enable (syncRecordButton / selectPane).
      btn!.disabled = false;
      fireEvent.click(btn!);
      expect(handlers[handlerName]).toHaveBeenCalledTimes(1);
    },
  );

  it.each(BUTTONS.map(([id]) => id))(
    "#%s renders without a `disabled` attribute (App owns the DOM state)",
    (id) => {
      renderInputBar();
      const btn = document.getElementById(id) as HTMLButtonElement;
      expect(btn.hasAttribute("disabled")).toBe(false);
    },
  );
});

// #input-bar's `direct` class mirrors the original renderMode's
// `$("input-bar").classList.toggle("direct", direct)` (InputBar.tsx:80). App
// drives the `direct` prop; both arms must match the vanilla markup.
describe("InputBar .direct class (renderMode parity)", () => {
  function renderWithDirect(direct: boolean) {
    render(
      <InputBar
        keyBar={null}
        modeIndicator={null}
        draft={null}
        direct={direct}
        recordBtnRef={createRef<HTMLButtonElement>()}
        sendBtnRef={createRef<HTMLButtonElement>()}
        onRecord={() => {}}
        onSend={() => {}}
      />,
    );
    return document.getElementById("input-bar") as HTMLDivElement;
  }

  it("adds `direct` to #input-bar when direct is true", () => {
    expect(renderWithDirect(true).className).toBe("input-bar direct");
  });

  it("uses the plain `input-bar` class when direct is false", () => {
    expect(renderWithDirect(false).className).toBe("input-bar");
  });
});
