// Agentry's service worker. Browsers offer to install the page as an app
// only when one handles its fetches. It keeps the bundles under /assets/,
// whose names carry their hash so they never change, and nothing else: the
// API, the terminals and the pages always go to the server, so no token is
// ever cached and a new version shows on the next load.

const CACHE = "agentry-assets";
const KEPT = 200; // about two versions' bundles; the oldest go first

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.origin !== self.location.origin || !url.pathname.startsWith("/assets/") || url.search) {
    return; // the browser fetches it as if there were no service worker
  }
  event.respondWith(fromCache(event.request));
});

async function fromCache(request) {
  const cache = await caches.open(CACHE);
  const kept = await cache.match(request);
  if (kept) return kept;
  const response = await fetch(request);
  if (response.ok) {
    await cache.put(request, response.clone());
    const keys = await cache.keys(); // oldest first
    await Promise.all(keys.slice(0, Math.max(0, keys.length - KEPT)).map((key) => cache.delete(key)));
  }
  return response;
}
