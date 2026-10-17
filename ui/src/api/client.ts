// Talking to `agentry serve`. The token comes from the link the server
// printed: the page keeps it in this browser and sends it on every call,
// and takes it out of the address bar so it isn't shared by accident.

const tokenKey = "agentry-token";

export const token: string = (() => {
  const query = new URLSearchParams(location.search);
  const fromLink = query.get("token");
  if (fromLink) {
    query.delete("token"); // the rest of the address, such as ?view=board, stays
    const rest = query.toString();
    history.replaceState(null, "", location.pathname + (rest ? "?" + rest : "") + location.hash);
  }
  try {
    if (fromLink) localStorage.setItem(tokenKey, fromLink);
    return fromLink || localStorage.getItem(tokenKey) || "";
  } catch {
    return fromLink || "";
  }
})();

export class SignedOutError extends Error {
  constructor() {
    super("Not signed in. Open the link `agentry serve` printed.");
  }
}

export async function api<T>(method: "GET" | "POST" | "PUT" | "DELETE", path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method,
    signal,
    headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 401) throw new SignedOutError();
  if (!response.ok) throw new Error((await response.text()).trim() || response.statusText);
  return response.json() as Promise<T>;
}

// Websockets can't send headers, so the terminal's carries the token in its address.
function socketURL(path: string, query = ""): string {
  const scheme = location.protocol === "https:" ? "wss://" : "ws://";
  return `${scheme}${location.host}${path}?token=${encodeURIComponent(token)}${query}`;
}

export function terminalURL(taskId: string): string {
  return socketURL(`/api/tasks/${encodeURIComponent(taskId)}/terminal`);
}

// Your own Claude session beside a task, in its worktree; started on the way.
export function helperTerminalURL(taskId: string, theme: "light" | "dark"): string {
  return socketURL(`/api/tasks/${encodeURIComponent(taskId)}/claude/terminal`, `&theme=${theme}`);
}

// The Claude session that turns what you say into tasks for the coordinator.
// It starts in the page's theme, and restarts in a new one, keeping the
// conversation, when the address's theme changes.
export function deskTerminalURL(theme: "light" | "dark"): string {
  return socketURL("/api/desk/terminal", `&theme=${theme}`);
}
