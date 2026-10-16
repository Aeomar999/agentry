// Monaco, the editor VS Code is built on, with only what Agentry shows:
// the editor itself, find (Ctrl+F), and highlighting for TOML (Monaco's ini
// grammar reads it well) and Markdown. Loaded on demand, in its own bundle,
// by the pages that show a file.

import * as monaco from "monaco-editor/esm/vs/editor/editor.api";
import "monaco-editor/esm/vs/editor/contrib/find/browser/findController";
import "monaco-editor/esm/vs/basic-languages/ini/ini.contribution";
import "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";

self.MonacoEnvironment = { getWorker: () => new EditorWorker() };

// The editor's colours come from the same tokens as the rest of the page.
// Monaco takes only #rrggbb, and the built CSS may say #fff or rgb(...),
// so each goes through a canvas, which names any colour as #rrggbb.
const canvas = document.createElement("canvas").getContext("2d")!;

function token(name: string): string {
  canvas.fillStyle = "#000000";
  canvas.fillStyle = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return canvas.fillStyle;
}

export function defineTheme(): string {
  const light = document.documentElement.classList.contains("light");
  monaco.editor.defineTheme("agentry", {
    base: light ? "vs" : "vs-dark",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": token("--color-surface-0"),
      "editor.foreground": token("--color-ink-primary"),
      "editorLineNumber.foreground": token("--color-ink-faint"),
      "editorLineNumber.activeForeground": token("--color-ink-secondary"),
      "editor.lineHighlightBackground": token("--color-surface-1"),
      "editor.selectionBackground": token("--color-surface-3"),
      "editorWidget.background": token("--color-surface-2"),
      "editorWidget.border": token("--color-divider-2"),
      "scrollbarSlider.background": token("--color-divider-2") + "99",
      "widget.shadow": "#00000000", // no shadows anywhere
    },
  });
  return "agentry";
}

export { monaco };
