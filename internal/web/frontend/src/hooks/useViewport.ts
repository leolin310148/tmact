import { useEffect, useRef } from "react";

// useViewport — 1:1 port of app.js fitViewport / scheduleFitViewport.
//
// The soft keyboard shrinks the visual viewport but not the layout viewport.
// Keep the flex shell sized to the visible viewport and cancel Safari's
// document-level focus scroll; pane output has its own scroll container, so
// resizing must not force it to the captured tmux buffer's blank tail.
//
// positionRecOverlay is injected (from useVoice) and called on every fit, exactly
// like app.js where positionRecOverlay() runs inside fitViewport().

export interface UseViewportDeps {
  positionRecOverlay: () => void;
}

export function useViewport({ positionRecOverlay }: UseViewportDeps): void {
  // Hold the latest positionRecOverlay in a ref so the listener-registration
  // effect never re-runs when the injected callback identity changes, while
  // fitViewport still calls the current implementation (the original closed
  // over a single positionRecOverlay reference).
  const positionRef = useRef(positionRecOverlay);
  positionRef.current = positionRecOverlay;

  useEffect(() => {
    function fitViewport() {
      const vv = window.visualViewport;
      if (!vv) return;
      document.documentElement.style.setProperty("--tmact-vvh", vv.height + "px");
      window.scrollTo(0, 0);
      positionRef.current();
    }
    function scheduleFitViewport() {
      fitViewport();
      requestAnimationFrame(fitViewport);
      setTimeout(fitViewport, 80);
      setTimeout(fitViewport, 260);
    }
    if (window.visualViewport) {
      const vv = window.visualViewport;
      vv.addEventListener("resize", scheduleFitViewport);
      vv.addEventListener("scroll", fitViewport);
      window.addEventListener("orientationchange", scheduleFitViewport);
      document.addEventListener("focusin", scheduleFitViewport);
      fitViewport();
      return () => {
        vv.removeEventListener("resize", scheduleFitViewport);
        vv.removeEventListener("scroll", fitViewport);
        window.removeEventListener("orientationchange", scheduleFitViewport);
        document.removeEventListener("focusin", scheduleFitViewport);
      };
    }
    return undefined;
    // Empty dep array: register once on mount (mirrors app.js top-level
    // registration). positionRecOverlay is read through positionRef so changes
    // to it do not re-register listeners.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}
