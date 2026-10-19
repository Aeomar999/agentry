// Pushes to this device: the browser subscribes with the server's VAPID key
// through the service worker (public/sw.js), and the server keeps the
// subscription (internal/web/push_api.go). What a push does when it comes
// is the service worker's.

import { api, ApiError } from "@/api/client";
import type { PushStatus } from "@/api/types";

// browserReason says why this browser can't take pushes here, or "" when it
// can. Browsers have no PushManager on a page that isn't secure, and some
// have none at all.
export function browserReason(): string {
  if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
    return "This browser can't receive pushes here. Open Agentry at its https address, in a browser that has notifications.";
  }
  if (Notification.permission === "denied") {
    return "Notifications from Agentry are blocked in this browser. Allow them in its site settings, then reopen this.";
  }
  return "";
}

// thisDevice is this browser's subscription, or null for none.
export async function thisDevice(): Promise<PushSubscription | null> {
  const registration = await navigator.serviceWorker.getRegistration();
  return (await registration?.pushManager.getSubscription()) ?? null;
}

// subscribe asks to notify, subscribes with the server's key and gives the
// server the subscription.
export async function subscribe(publicKey: string): Promise<PushStatus> {
  if ((await Notification.requestPermission()) !== "granted") {
    throw new Error("Notifications weren't allowed.");
  }
  await navigator.serviceWorker.register("/sw.js"); // the one the page has, or again if it was refused
  const registration = await navigator.serviceWorker.ready;
  const subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: fromBase64url(publicKey) });
  return api<PushStatus>("POST", "/api/push/subscriptions", subscription.toJSON());
}

// unsubscribe stops the pushes to this browser, at the server and in it.
export async function unsubscribe(subscription: PushSubscription): Promise<void> {
  await api("DELETE", `/api/push/subscriptions?endpoint=${encodeURIComponent(subscription.endpoint)}`).catch((failure: Error) => {
    if (!(failure instanceof ApiError && failure.status === 404)) throw failure; // 404: the server had forgotten it already
  });
  await subscription.unsubscribe();
}

function fromBase64url(text: string): Uint8Array<ArrayBuffer> {
  const binary = atob(text.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}
