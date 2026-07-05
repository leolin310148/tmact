// CACHE_NAME's suffix is rewritten by the Go server to a content hash of the
// embedded static assets, so any change under internal/web/static/ buys a
// fresh cache automatically — no manual bump needed. The literal vDEV here is
// only what you see when reading the file on disk.
const CACHE_NAME = "tmact-app-shell-vDEV";

// Stable, known-path shell entries to precache on install. The React build emits
// content-hashed JS/CSS under /assets/ whose names change every build, so they
// cannot be enumerated here; the fetch handler caches them opportunistically
// (network-first) once they are first requested. Offline support is preserved:
// after one online load, "/" + its hashed assets are all in the cache.
const APP_SHELL_URLS = [
  "/",
  "/index.html",
  "/manifest.json",
  "/icons/icon-180.png",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
];
const APP_SHELL_PATHS = new Set(APP_SHELL_URLS);

// A request is part of the app shell if it is a known stable path OR a hashed
// build asset under /assets/.
function isShellPath(pathname) {
  return APP_SHELL_PATHS.has(pathname) || pathname.startsWith("/assets/");
}

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

  if (!isShellPath(url.pathname)) {
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

self.addEventListener("push", (event) => {
  let data = {};

  if (event.data) {
    try {
      data = event.data.json();
    } catch (err) {
      data = { body: event.data.text() };
    }
  }

  if (!data || typeof data !== "object") {
    data = { body: String(data || "") };
  }

  const title = data.title || "tmact";
  const options = {
    body: data.body || "",
    icon: "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    tag: data.tag || "tmact-status",
    renotify: Boolean(data.tag),
    data: { url: normalizeAppURL(data.url) },
  };

  event.waitUntil(
    self.registration.showNotification(title, options).then(() =>
      self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
        clients.forEach((client) => {
          client.postMessage({ type: "PUSH_RECEIVED", payload: data });
        });
      }),
    ),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const targetURL = normalizeAppURL(event.notification.data && event.notification.data.url);

  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      const sameOriginClient = clients.find((client) => {
        try {
          return new URL(client.url).origin === self.location.origin;
        } catch (err) {
          return false;
        }
      });

      if (sameOriginClient) {
        return sameOriginClient.focus();
      }

      return self.clients.openWindow(targetURL);
    }),
  );
});

function normalizeAppURL(rawURL) {
  if (typeof rawURL !== "string" || rawURL.trim() === "") {
    return "/";
  }
  try {
    const url = new URL(rawURL, self.location.origin);
    if (url.origin !== self.location.origin) {
      return "/";
    }
    return url.pathname + url.search + url.hash;
  } catch (err) {
    return "/";
  }
}
