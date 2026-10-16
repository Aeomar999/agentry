import { useEffect, useRef } from "react";
import { defineTheme, monaco } from "@/lib/monaco";
import type { CodeViewerProps } from "./CodeViewer";

// The Monaco half of CodeViewer, in its own bundle.
export default function MonacoView({ text, language, maxHeight = 640 }: CodeViewerProps) {
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const editor = monaco.editor.create(host.current!, {
      value: text,
      language,
      theme: defineTheme(),
      readOnly: true,
      domReadOnly: true,
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      wordWrap: language === "markdown" ? "on" : "off",
      fontFamily: getComputedStyle(document.documentElement).getPropertyValue("--font-mono"),
      fontSize: 12.5,
      lineHeight: 20,
      renderLineHighlight: "none",
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      padding: { top: 10, bottom: 10 },
      scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8, useShadows: false },
      contextmenu: false,
    });
    // Grow with the text, up to maxHeight.
    const fit = () => (host.current!.style.height = `${Math.min(maxHeight, editor.getContentHeight())}px`);
    const sizing = editor.onDidContentSizeChange(fit);
    fit();

    // Follow the page's theme when it changes.
    const themeWatch = new MutationObserver(() => monaco.editor.setTheme(defineTheme()));
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });

    return () => {
      themeWatch.disconnect();
      sizing.dispose();
      editor.getModel()?.dispose();
      editor.dispose();
    };
  }, [text, language, maxHeight]);

  return <div ref={host} />;
}
