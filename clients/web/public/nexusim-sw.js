const CACHE_NAME = "nexusim-browser-shell-v1";
const PRECACHE_URLS = ["/", "/manifest.webmanifest", "/pwa-icon.svg"];
const NETWORK_ONLY_PREFIXES = ["/api/", "/ws"];
const NETWORK_ONLY_PATHS = ["/nexusim-shell-config.js", "/nexusim-sw.js"];

self.addEventListener("install", event => {
  event.waitUntil(
    caches
      .open(CACHE_NAME)
      .then(cache => cache.addAll(PRECACHE_URLS))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", event => {
  event.waitUntil(
    caches
      .keys()
      .then(names =>
        Promise.all(names.filter(name => name !== CACHE_NAME).map(name => caches.delete(name)))
      )
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", event => {
  const request = event.request;
  if (request.method !== "GET") {
    return;
  }
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || shouldBypassCache(url.pathname)) {
    return;
  }
  if (request.mode === "navigate") {
    event.respondWith(networkFirstNavigation(request));
    return;
  }
  event.respondWith(cacheFirst(request));
});

function shouldBypassCache(pathname) {
  return (
    NETWORK_ONLY_PATHS.includes(pathname) ||
    NETWORK_ONLY_PREFIXES.some(prefix => pathname.startsWith(prefix))
  );
}

async function networkFirstNavigation(request) {
  const cache = await caches.open(CACHE_NAME);
  try {
    const response = await fetch(request);
    if (response.ok) {
      await cache.put("/", response.clone());
    }
    return response;
  } catch {
    const cached = await cache.match("/") || await cache.match(request);
    if (cached) {
      return cached;
    }
    throw new Error("navigation unavailable");
  }
}

async function cacheFirst(request) {
  const cache = await caches.open(CACHE_NAME);
  const cached = await cache.match(request);
  if (cached) {
    return cached;
  }
  const response = await fetch(request);
  if (response.ok) {
    await cache.put(request, response.clone());
  }
  return response;
}
