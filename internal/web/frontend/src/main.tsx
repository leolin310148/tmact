import { createRoot } from "react-dom/client";
import App from "./components/App";
import "./app.css";
import { initFrontendLogging } from "./lib/frontendLog";
import { initSplitShell, wasSplitActive } from "./split";
import { isMobile } from "./lib/dom";
import { inSplitSlot } from "./lib/slot";

// ?view=split mounts the desktop split shell instead of the app. A ?slot=N
// wins over it (a slot iframe must always render the app — never a nested
// shell), so the shell's own iframes can't recurse. A bare "/" on desktop
// (the PWA start_url) restores split view when the user was last in it —
// the shell records that in localStorage and its exit button clears it.
const params = new URLSearchParams(window.location.search);
const desktop = typeof window.matchMedia === "function" && !isMobile();
const splitView =
  !params.has("slot") &&
  (params.get("view") === "split" ||
    (!params.has("view") && desktop && wasSplitActive()));

// NOTE: no <StrictMode>. The original vanilla app has no double-invocation, and
// several hooks own imperative resources (WebSocket, MediaRecorder, timers) via
// refs; StrictMode's dev-only double effect mount would open them twice. We keep
// dev behavior identical to production for faithful parity testing.
// Slot documents get a root attribute so app.css can keep desktop behaviors
// (e.g. the draft box in direct mode) despite the phone-width breakpoint the
// narrow column triggers. Set before mount so the first paint is correct.
if (inSplitSlot) {
  document.documentElement.dataset.splitSlot = "1";
}

const rootEl = document.getElementById("root");
if (rootEl) {
  initFrontendLogging();
  if (splitView) {
    initSplitShell(rootEl);
  } else {
    createRoot(rootEl).render(<App />);
  }
}

// PWA service worker — production only. In Vite dev, a service worker can
// control the page and interfere with HMR/module updates.
if (import.meta.env.PROD && "serviceWorker" in navigator) {
  let refreshing = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (refreshing) return;
    refreshing = true;
    window.location.reload();
  });

  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register("/sw.js")
      .then((registration) => {
        void registration.update();
      })
      .catch(() => {});
  });
} else if (import.meta.env.DEV && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker
      .getRegistrations()
      .then((registrations) => Promise.all(registrations.map((registration) => registration.unregister())))
      .then(() => {
        if (navigator.serviceWorker.controller) {
          window.location.reload();
        }
      })
      .catch(() => {});
  });
}
