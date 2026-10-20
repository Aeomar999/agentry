import { useEffect, useRef } from "react";
import { defineTheme, monaco } from "@/lib/monaco";
import type { CodeViewerProps } from "./CodeViewer";

// The Monaco half of CodeViewer, in its own bundle. The editor is made once
// per language; new text from outside is put into it, and typing in it
// (when onChange is given) goes out through onChange.
export default function MonacoView({
  text,
  language,
  maxHeight = 640,
  minHeight = 0,
  onChange,
  readOnly = false,
  placeholder,
  tabFocusMode = false,
  alwaysConsumeMouseWheel = true,
}: CodeViewerProps) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const changed = useRef(onChange);
  changed.current = onChange;
  const editable = onChange !== undefined;

  useEffect(() => {
    const instance = monaco.editor.create(host.current!, {
      value: text,
      language,
      theme: defineTheme(),
      readOnly: !editable,
      domReadOnly: !editable,
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      wordWrap: language === "markdown" ? "on" : "off",
      fontFamily: getComputedStyle(document.documentElement).getPropertyValue("--font-mono"),
      fontSize: 12.5,
      lineHeight: 20,
      renderLineHighlight: editable ? "line" : "none",
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      padding: { top: 10, bottom: 10 },
      tabFocusMode,
      scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8, useShadows: false, alwaysConsumeMouseWheel },
      contextmenu: false,
    });
    editor.current = instance;
    // Grow with the text, between minHeight and maxHeight.
    const fit = () => (host.current!.style.height = `${Math.max(minHeight, Math.min(maxHeight, instance.getContentHeight()))}px`);
    const sizing = instance.onDidContentSizeChange(fit);
    const typing = instance.onDidChangeModelContent(() => changed.current?.(instance.getValue()));
    fit();

    // Follow the page's theme when it changes.
    const themeWatch = new MutationObserver(() => monaco.editor.setTheme(defineTheme()));
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });

    return () => {
      themeWatch.disconnect();
      typing.dispose();
      sizing.dispose();
      instance.getModel()?.dispose();
      instance.dispose();
      editor.current = null;
    };
  }, [language, editable, maxHeight, minHeight, tabFocusMode, alwaysConsumeMouseWheel]); // made once; new text arrives below

  useEffect(() => {
    if (editor.current && editor.current.getValue() !== text) editor.current.setValue(text);
  }, [text]);

  useEffect(() => {
    editor.current?.updateOptions({ readOnly: !editable || readOnly, placeholder });
  }, [editable, readOnly, placeholder]);

  return <div ref={host} className={editable ? "rounded-md border border-divider focus-within:border-focus" : undefined} />;
}
