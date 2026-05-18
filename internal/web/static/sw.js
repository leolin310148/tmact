const CACHE_NAME = "tmact-app-shell-v4";
const APP_SHELL_URLS = [
  "/",
  "/index.html",
  "/manifest.json",
  "/icons/icon-180.png",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
];
const APP_SHELL_PATHS = new Set(APP_SHELL_URLS);

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => cache.addAll(APP_SHELL_URLS))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((names) => Promise.all(
        names
          .filter((name) => name !== CACHE_NAME)
          .map((name) => caches.delete(name)),
      ))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);

  if (
    request.method !== "GET" ||
    url.origin !== self.location.origin ||
    url.pathname.startsWith("/api/") ||
    url.pathname.startsWith("/ws/")
  ) {
    return;
  }

  if (!APP_SHELL_PATHS.has(url.pathname)) {
    return;
  }

  event.respondWith(
    caches.match(request)
      .then((cached) => cached || fetch(request)),
  );
});
