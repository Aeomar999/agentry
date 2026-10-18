// Installing the page as an app. The browser offers it (beforeinstallprompt)
// only on a secure page, http://localhost or https, that has the manifest
// and a service worker (public/sw.js). The page keeps the offer for its
// Install app button. Opened over plain http at a network address it can't
// be installed, and the button says where it can.

import { useSyncExternalStore } from "react";

interface InstallPromptEvent extends Event {
  prompt(): Promise<void>;
}

export type InstallState = "installed" | "ready" | "insecure" | "unavailable";

const standalone = matchMedia("(display-mode: standalone)");
let offer: InstallPromptEvent | null = null;
let installed = standalone.matches;
const listeners = new Set<() => void>();
const changed = () => listeners.forEach((listener) => listener());

// watchInstall registers the service worker and listens for the offer, from
// the start: the browser may make it before the page renders.
export function watchInstall(): void {
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").catch(() => {}); // the page works without it, only not as an app
  }
  window.addEventListener("beforeinstallprompt", (event) => {
    event.preventDefault(); // the page's own button offers it
    offer = event as InstallPromptEvent;
    changed();
  });
  window.addEventListener("appinstalled", () => {
    offer = null;
    installed = true;
    changed();
  });
  standalone.addEventListener("change", () => {
    installed = installed || standalone.matches;
    changed();
  });
}

function installState(): InstallState {
  if (installed) return "installed";
  if (!window.isSecureContext) return "insecure";
  return offer ? "ready" : "unavailable";
}

export function useInstall(): InstallState {
  return useSyncExternalStore((listener) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  }, installState);
}

// install shows the browser's own install prompt. An offer prompts once; the
// browser makes a new one if you dismiss it.
export async function install(): Promise<void> {
  const shown = offer;
  offer = null;
  changed();
  await shown?.prompt();
}
