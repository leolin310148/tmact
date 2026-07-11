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

  const paneId = normalizePaneID(data.paneId || data.pane_id);
  const rawTag = typeof data.tag === "string" ? data.tag.trim() : "";
  const title = data.title || "tmact";
  const tag = normalizeNotificationTag(rawTag, paneId);
  const options = {
    body: data.body || "",
    icon: "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    tag,
    renotify: Boolean(rawTag),
    data: {
      url: normalizeAppURL(data.url, paneId),
      paneId,
      rawTag,
    },
  };

  event.waitUntil(
    closeSupersededNotifications(tag, paneId, rawTag)
      .then(() => self.registration.showNotification(title, options))
      .then(() => self.clients.matchAll({ type: "window", includeUncontrolled: true }))
      .then((clients) => {
        clients.forEach((client) => {
          client.postMessage({ type: "PUSH_RECEIVED", payload: data });
        });
      }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const notificationData = event.notification.data || {};
  const paneId = normalizePaneID(notificationData.paneId);
  const targetURL = normalizeAppURL(notificationData.url, paneId);

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
        sameOriginClient.postMessage({ type: "SELECT_PANE", paneId, url: targetURL });
        return sameOriginClient.focus();
      }

      return self.clients.openWindow(targetURL);
    }),
  );
});

function normalizeAppURL(rawURL, paneId) {
  let url;
  if (typeof rawURL !== "string" || rawURL.trim() === "") {
    url = new URL("/", self.location.origin);
  } else {
    try {
      url = new URL(rawURL, self.location.origin);
    } catch (err) {
      url = new URL("/", self.location.origin);
    }
  }
  if (url.origin !== self.location.origin) {
    url = new URL("/", self.location.origin);
  }
  if (paneId) {
    url.searchParams.set("pane", paneId);
  }
  return url.pathname + url.search + url.hash;
}

function normalizeNotificationTag(rawTag, paneId) {
  let tag = typeof rawTag === "string" ? rawTag.trim() : "";
  if (paneId) {
    const separator = paneId.indexOf("@%");
    const localPaneId = separator >= 0 ? paneId.slice(separator + 1) : paneId;
    const safePane = separator >= 0
      ? `${paneId.slice(0, separator)}-pane-${localPaneId.slice(1)}`
      : `pane-${localPaneId.slice(1)}`;
    const encodedPane = encodeURIComponent(paneId);
    if (tag) {
      tag = tag.split(encodedPane).join(safePane).split(paneId).join(safePane);
      // Existing hooks commonly emit tags such as claude-%60. Preserve that
      // convention while including the peer so equal local pane ids on two
      // machines cannot collapse each other's notifications.
      if (separator >= 0) {
        const encodedLocalPane = encodeURIComponent(localPaneId);
        tag = tag.split(encodedLocalPane).join(safePane).split(localPaneId).join(safePane);
        if (!tag.includes(safePane)) {
          tag = `${safePane}-${tag}`;
        }
      }
    }
    if (!tag) {
      tag = `tmact-${safePane}`;
    }
  }
  return sanitizeNotificationTag(tag || "tmact-status");
}

function sanitizeNotificationTag(tag) {
  return tag
    .replace(/[^A-Za-z0-9_-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "") || "tmact-status";
}

function closeSupersededNotifications(tag, paneId, rawTag) {
  if (!self.registration.getNotifications) {
    return Promise.resolve();
  }
  return self.registration.getNotifications()
    .then((notifications) => {
      notifications.forEach((notification) => {
        if (shouldCloseSupersededNotification(notification, tag, paneId, rawTag)) {
          notification.close();
        }
      });
    })
    .catch(() => {
      if (!tag) {
        return undefined;
      }
      return self.registration.getNotifications({ tag }).then((notifications) => {
        notifications.forEach((notification) => notification.close());
      }).catch(() => undefined);
    });
}

function shouldCloseSupersededNotification(notification, tag, paneId, rawTag) {
  if (!notification) {
    return false;
  }
  if (tag && notification.tag === tag) {
    return true;
  }
  if (!paneId) {
    return Boolean(rawTag && notification.tag === rawTag);
  }
  const federated = paneId.includes("@%");
  const encodedPaneId = encodeURIComponent(paneId);
  const legacyRawTag = `claude-${paneId}`;
  const legacyEncodedTag = `claude-${encodedPaneId}`;
  if (!federated && (
    notification.tag === legacyRawTag ||
    notification.tag === legacyEncodedTag ||
    (rawTag && notification.tag === rawTag)
  )) {
    return true;
  }
  const notificationData = notification.data || {};
  const notificationPaneId = normalizePaneID(notificationData.paneId || notificationData.pane_id);
  if (notificationPaneId) {
    return notificationPaneId === paneId;
  }
  // A legacy notification without pane metadata can only be matched safely by
  // raw tag for local panes. The same legacy tag may exist on several peers.
  return Boolean(!federated && rawTag && notificationData.rawTag === rawTag);
}

function normalizePaneID(rawPaneId) {
  if (typeof rawPaneId !== "string") {
    return "";
  }
  let paneId = rawPaneId.trim();
  if (paneId.includes("%25")) {
    try {
      paneId = decodeURIComponent(paneId);
    } catch (err) {
      return "";
    }
  }
  return /^(?:[A-Za-z0-9_.-]+@)?%[0-9]+$/.test(paneId) ? paneId : "";
}
