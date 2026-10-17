import { useEffect, useState } from "react";
import { defineTheme, languageFor, monaco } from "@/lib/monaco";

// Code highlighted by Monaco's colorizer, as the editor itself would show
// it, without an editor. In Monaco's own bundle, loaded when first needed.
// The colorizer escapes the text it is given, so its HTML is safe to show.
export default function MonacoColorize({ code, language }: { code: string; language: string }) {
  const [html, setHtml] = useState("");
  useEffect(() => {
    let current = true;
    monaco.editor.setTheme(defineTheme());
    const id = monaco.languages.getLanguages().some((l) => l.id === language) ? language : languageFor(`x.${language}`);
    monaco.editor.colorize(code, id, { tabSize: 2 }).then((colored) => current && setHtml(colored));
    return () => {
      current = false;
    };
  }, [code, language]);
  if (!html) return <>{code}</>;
  return <span dangerouslySetInnerHTML={{ __html: html }} />;
}
