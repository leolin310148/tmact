import { createRoot } from "react-dom/client";
import App from "./components/App";
import "./app.css";
import { initFrontendLogging } from "./lib/frontendLog";

// NOTE: no <StrictMode>. The original vanilla app has no double-invocation, and
// several hooks own imperative resources (WebSocket, MediaRecorder, timers) via
// refs; StrictMode's dev-only double effect mount would open them twice. We keep
// dev behavior identical to production for faithful parity testing.
const rootEl = document.getElementById("root");
if (rootEl) {
  initFrontendLogging();
  createRoot(rootEl).render(<App />);
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
