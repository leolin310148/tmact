const CACHE_NAME = "tmact-app-shell-v7";
const APP_SHELL_URLS = [
  "/",
  "/index.html",
  "/app.css",
  "/app.js",
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

// Network-first for the app shell: an online client always gets the freshly
// built UI, with the cache refreshed behind it so the app still loads when
// offline. The earlier cache-first handler pinned clients to a stale
// index.html — a new build never showed up without bumping CACHE_NAME.
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
    fetch(request)
      .then((response) => {
        if (response && response.ok) {
          const copy = response.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(request, copy));
        }
        return response;
      })
      .catch(() => caches.match(request)),
  );
});
