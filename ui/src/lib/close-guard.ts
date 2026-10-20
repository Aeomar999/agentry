import { useEffect, useRef, useState } from "react";
import { useBlocker } from "react-router-dom";

// useCloseGuard keeps a panel's unsaved changes from going without a
// question (docs/design/custom-teams.md, decisions 7 and 11). Close, Cancel
// and Esc call ask: with no changes the panel closes at once; with changes
// asking turns on, and the panel asks in its footer. Browser Back with
// changes is held, so the panel's history entry stays, and asks the same.
// Following a link with changes is held too; opening then says where the
// link goes, so the panel can ask about that instead. Reload and closing
// the tab get the browser's own prompt. While busy (a save) nothing asks
// and Back stays put; the save's answer decides.
export interface CloseGuard {
  asking: boolean;
  opening: string; // the path of the link that was held while asking, "" for closing
  ask: () => void; // Close, Cancel and Esc; Esc again while asking is Keep editing
  keepEditing: () => void; // puts focus back where it was when ask was called
  leave: () => void; // closes for good: Discard, or after a save
}

export function useCloseGuard(changed: boolean, busy: boolean, onClose: () => void): CloseGuard {
  const [asked, setAsked] = useState(false);
  const leaving = useRef(false);
  const from = useRef<Element | null>(null); // where focus was when ask was called
  const [opening, setOpening] = useState("");
  const blocker = useBlocker(({ nextLocation, historyAction }) => {
    const hold = changed && !leaving.current;
    if (hold && !from.current) from.current = document.activeElement; // asking from Close already knows
    if (hold) setOpening(historyAction === "PUSH" ? nextLocation.pathname : "");
    return hold;
  });
  const asking = asked || (blocker.state === "blocked" && !busy);

  useEffect(() => {
    if (blocker.state !== "blocked" || !busy) return;
    from.current = null;
    blocker.reset();
  }, [blocker, busy]);

  useEffect(() => {
    if (!changed) return;
    const prompt = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", prompt);
    return () => window.removeEventListener("beforeunload", prompt);
  }, [changed]);

  // Back to where focus was, or to the element with its id: the question
  // took the place of the footer's buttons, which came back as new ones.
  useEffect(() => {
    if (asking || !from.current) return;
    const was = from.current;
    from.current = null;
    const back = was.isConnected ? was : was.id ? document.getElementById(was.id) : null;
    if (back instanceof HTMLElement) back.focus();
  }, [asking]);

  // leave runs once: a second Cancel before the first lands would go back twice.
  function leave() {
    if (leaving.current) return;
    leaving.current = true;
    if (blocker.state === "blocked") blocker.proceed();
    else onClose();
  }

  function keepEditing() {
    setAsked(false);
    if (blocker.state === "blocked") blocker.reset();
  }

  function ask() {
    if (busy) return;
    if (asking) return keepEditing();
    if (!changed) return leave();
    from.current = document.activeElement;
    setAsked(true);
  }

  return { asking, opening: blocker.state === "blocked" ? opening : "", ask, keepEditing, leave };
}
