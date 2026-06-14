/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const frontendBuildTime = new Date().toISOString();

// The Go server embeds internal/web/static via go:embed and serves it verbatim
// with http.FileServer. Vite therefore builds INTO ../static:
//   - index.html, sw.js, manifest.json, icons/* land at the static root
//     (public/* is copied verbatim), so /sw.js and /manifest.json keep working.
//   - hashed JS/CSS land under static/assets/*.
// The Go asset-hash walks every file under static/, so any asset change still
// flips the service-worker CACHE_NAME automatically — no manual bump.
export default defineConfig({
  plugins: [react()],
  base: "/",
  define: {
    __TMACT_FRONTEND_BUILD__: JSON.stringify(frontendBuildTime),
  },
  build: {
    outDir: "../static",
    emptyOutDir: true,
    assetsDir: "assets",
    sourcemap: false,
    // The only chunks over 500 kB are mermaid's lazy diagram bundles
    // (mermaid-parser ~600 kB, cytoscape ~435 kB), loaded on demand behind
    // import("mermaid") — never on first paint. Raise the limit so the build
    // warning flags real first-paint regressions, not mermaid's heft.
    chunkSizeWarningLimit: 700,
  },
  server: {
    port: 5234,
    // In dev, proxy the API + WebSocket to a running statusd web server.
    // statusd's default web-addr is 127.0.0.1:7890 (statusd.DefaultWebAddr);
    // override with TMACT_STATUSD=host:port.
    proxy: (() => {
      const target = process.env.TMACT_STATUSD ?? "127.0.0.1:7890";
      return {
        "/api": { target: `http://${target}`, changeOrigin: true },
        "/ws": { target: `ws://${target}`, ws: true },
      };
    })(),
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
  },
});
