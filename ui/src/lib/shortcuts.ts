import { useEffect, useRef, useState } from "react";

// Every keyboard shortcut, in one list. The ? overlay shows exactly this
// list, so a shortcut that isn't here doesn't exist.

export interface Shortcut {
  keys: string[]; // pressed in order: ["g", "t"] is G then T
  label: string;
  group: "Anywhere" | "Go to" | "Lists";
}

export const modKey = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl";

export const SHORTCUTS: Shortcut[] = [
  { keys: [`${modKey} K`], label: "Command palette", group: "Anywhere" },
  { keys: [`${modKey} J`], label: "Show or hide the terminal", group: "Anywhere" },
  { keys: ["?"], label: "Show these shortcuts", group: "Anywhere" },
  { keys: ["n"], label: "New task", group: "Anywhere" },
  { keys: ["/"], label: "Search this page", group: "Anywhere" },
  { keys: ["Esc"], label: "Close a palette or overlay", group: "Anywhere" },
  { keys: ["g", "t"], label: "Tasks", group: "Go to" },
  { keys: ["g", "a"], label: "Agents", group: "Go to" },
  { keys: ["g", "p"], label: "Policy", group: "Go to" },
  { keys: ["j"], label: "Next row", group: "Lists" },
  { keys: ["k"], label: "Previous row", group: "Lists" },
  { keys: ["Enter"], label: "Open the row", group: "Lists" },
];

// isTyping is true while focus is in a field or the agent's terminal, where
// keys are text, not shortcuts.
export function isTyping(event: KeyboardEvent): boolean {
  const target = event.target as HTMLElement | null;
  if (!target) return false;
  return target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

// useKeys calls the handler for a single key or a two-key chord ("g t"),
// pressed anywhere outside a field. The handler map may change each render.
export function useKeys(handlers: Record<string, () => void>): void {
  const latest = useRef(handlers);
  latest.current = handlers;

  useEffect(() => {
    let pending = "";
    let timer = 0;
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey || isTyping(event)) return;
      if (event.key === "Enter" && event.target !== document.body) return; // a focused button or row handles its own Enter
      const key = pending ? `${pending} ${event.key}` : event.key;
      window.clearTimeout(timer);
      pending = "";
      if (latest.current[key]) {
        event.preventDefault();
        latest.current[key]();
      } else if (Object.keys(latest.current).some((chord) => chord.startsWith(event.key + " "))) {
        pending = event.key;
        timer = window.setTimeout(() => (pending = ""), 1200);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.clearTimeout(timer);
    };
  }, []);
}

// useRowCursor moves a cursor over a list with j and k, and opens the row
// under it with Enter. It returns the cursor's index, -1 before any key.
export function useRowCursor(count: number, open: (index: number) => void): number {
  const [cursor, setCursor] = useState(-1);
  useKeys({
    j: () => setCursor((at) => Math.min(count - 1, at + 1)),
    k: () => setCursor((at) => Math.max(0, at - 1)),
    Enter: () => cursor >= 0 && cursor < count && open(cursor),
  });
  return Math.min(cursor, count - 1);
}
