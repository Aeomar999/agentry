// Agentry's service worker. Browsers offer to install the page as an app
// only when one handles its fetches. It keeps the bundles under /assets/,
// whose names carry their hash so they never change, and nothing else: the
// API, the terminals and the pages always go to the server, so no token is
// ever cached and a new version shows on the next load. It also shows the
// pushes the server sends when something new waits for you
// (internal/webpush), and opens its task when one is tapped.

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

self.addEventListener("push", (event) => {
  const push = pushed(event.data);
  event.waitUntil(self.registration.showNotification(push.title, { body: push.body, tag: push.tag, data: { url: push.url } }));
});

// pushed is what a push says to show, or a plain "Agentry" when it says
// nothing that can be read.
function pushed(data) {
  let message = {};
  try {
    message = data?.json() ?? {};
  } catch {
    // not JSON: shown as a plain notification
  }
  const text = (value) => (typeof value === "string" ? value : "");
  return { title: text(message.title) || "Agentry", body: text(message.body), tag: text(message.tag) || undefined, url: text(message.url) };
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(open(pageOf(event.notification.data?.url)));
});

// pageOf is the address of a page of this app that url names, or the app's
// start for any other: a push only ever opens Agentry.
function pageOf(url) {
  if (typeof url !== "string" || !url.startsWith("/") || url.startsWith("//")) return self.location.origin + "/";
  const page = new URL(url, self.location.origin);
  return page.origin === self.location.origin ? page.href : self.location.origin + "/";
}

// open shows page in a window of the app that is open, or in a new one.
// Only a window this worker controls can be sent to another page, and it
// is one of this origin.
async function open(page) {
  const [mine] = await self.clients.matchAll({ type: "window" });
  if (!mine) return self.clients.openWindow(page);
  await mine.focus();
  return mine.navigate(page);
}
