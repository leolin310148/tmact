import { createRoot } from "react-dom/client";
import App from "./components/App";
import "./app.css";

// NOTE: no <StrictMode>. The original vanilla app has no double-invocation, and
// several hooks own imperative resources (WebSocket, MediaRecorder, timers) via
// refs; StrictMode's dev-only double effect mount would open them twice. We keep
// dev behavior identical to production for faithful parity testing.
const rootEl = document.getElementById("root");
if (rootEl) {
  createRoot(rootEl).render(<App />);
}

// PWA service worker — fire-and-forget, errors ignored (parity with original app.js).
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  });
}
