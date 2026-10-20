import { useCallback, useEffect, useRef } from "react";

// useMarkedText gives the text last marked on the page, kept after the mark
// is gone, as a click on a button can clear it before the click lands. Each
// mark is given once, so a later click doesn't start from an old one.
export function useMarkedText(): () => string {
  const last = useRef("");
  useEffect(() => {
    const keep = () => {
      const text = document.getSelection()?.toString() ?? "";
      if (text.trim()) last.current = text;
    };
    document.addEventListener("selectionchange", keep);
    return () => document.removeEventListener("selectionchange", keep);
  }, []);
  return useCallback(() => {
    const text = last.current;
    last.current = "";
    return text;
  }, []);
}
