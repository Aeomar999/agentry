import { useEffect, useRef } from "react";
import { defineTheme, languageFor, monaco } from "@/lib/monaco";

export interface FileViewProps {
  path: string;
  now: string;
  before?: string; // given: show the changes since it, side by side
}

// A workspace file in Monaco, read only, filling its box: the file as it is,
// or its diff against how it was when the task started. In its own bundle.
export default function MonacoDiff({ path, now, before }: FileViewProps) {
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const options = {
      theme: defineTheme(),
      readOnly: true,
      domReadOnly: true,
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      fontFamily: getComputedStyle(document.documentElement).getPropertyValue("--font-mono"),
      fontSize: 12.5,
      lineHeight: 20,
      contextmenu: false,
      scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8, useShadows: false },
    };
    const language = languageFor(path);
    const models: monaco.editor.ITextModel[] = [];
    let editor: monaco.editor.IEditor;
    if (before === undefined) {
      const model = monaco.editor.createModel(now, language);
      models.push(model);
      editor = monaco.editor.create(host.current!, { ...options, model });
    } else {
      const original = monaco.editor.createModel(before, language);
      const modified = monaco.editor.createModel(now, language);
      models.push(original, modified);
      const diff = monaco.editor.createDiffEditor(host.current!, { ...options, renderSideBySide: true, originalEditable: false });
      diff.setModel({ original, modified });
      editor = diff;
    }
    const themeWatch = new MutationObserver(() => monaco.editor.setTheme(defineTheme()));
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => {
      themeWatch.disconnect();
      editor.dispose();
      models.forEach((model) => model.dispose());
    };
  }, [path, now, before]);

  return <div ref={host} className="h-full w-full" />;
}
