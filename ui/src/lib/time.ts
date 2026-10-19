// How times read on the page: short, and relative where recency matters.

export function clockTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function dateTime(iso: string): string {
  return new Date(iso).toLocaleString([], { dateStyle: "medium", timeStyle: "short" });
}

// When a usage limit hit at atIso resets: "usage limit, resets ~03:00",
// with the date when that isn't the day it was hit, or "usage limit, reset
// time unknown".
export function limitText(resetsIso: string | undefined, atIso: string): string {
  if (!resetsIso) return "usage limit, reset time unknown";
  const resets = new Date(resetsIso);
  const sameDay = resets.toDateString() === new Date(atIso).toDateString();
  const time = resets.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  return `usage limit, resets ~${sameDay ? time : `${resets.toLocaleDateString([], { month: "short", day: "numeric" })} ${time}`}`;
}

export function timeAgo(iso: string, now = Date.now()): string {
  const seconds = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

// How long something took, or has taken so far: 0.4s, 12s, 3m 05s, 1h 04m.
export function duration(fromIso: string, toIso?: string, now = Date.now()): string {
  const ms = Math.max(0, (toIso ? new Date(toIso).getTime() : now) - new Date(fromIso).getTime());
  if (ms < 10_000) return `${(ms / 1000).toFixed(1)}s`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const pad = (n: number) => String(n).padStart(2, "0");
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${pad(seconds % 60)}s`;
  return `${Math.floor(seconds / 3600)}h ${pad(Math.floor(seconds / 60) % 60)}m`;
}
