// Monaco, the editor VS Code is built on, with only what Agentry shows: the
// editor and its diff view, find (Ctrl+F), and highlighting for TOML
// (Monaco's ini grammar reads it well), Markdown and the common languages
// of a workspace. Loaded on demand, in its own bundle, by the pages that
// show a file.

import * as monaco from "monaco-editor/esm/vs/editor/editor.api";
import "monaco-editor/esm/vs/editor/contrib/find/browser/findController";
import "monaco-editor/esm/vs/base/browser/ui/codicons/codicon/codicon.css"; // its icons: fold arrows, find, diff
import "monaco-editor/esm/vs/base/browser/ui/codicons/codicon/codicon-modifiers.css";
import "monaco-editor/esm/vs/basic-languages/ini/ini.contribution";
import "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution";
import "monaco-editor/esm/vs/basic-languages/go/go.contribution";
import "monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution";
import "monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution";
import "monaco-editor/esm/vs/basic-languages/python/python.contribution";
import "monaco-editor/esm/vs/basic-languages/rust/rust.contribution";
import "monaco-editor/esm/vs/basic-languages/java/java.contribution";
import "monaco-editor/esm/vs/basic-languages/kotlin/kotlin.contribution";
import "monaco-editor/esm/vs/basic-languages/swift/swift.contribution";
import "monaco-editor/esm/vs/basic-languages/cpp/cpp.contribution";
import "monaco-editor/esm/vs/basic-languages/csharp/csharp.contribution";
import "monaco-editor/esm/vs/basic-languages/ruby/ruby.contribution";
import "monaco-editor/esm/vs/basic-languages/php/php.contribution";
import "monaco-editor/esm/vs/basic-languages/css/css.contribution";
import "monaco-editor/esm/vs/basic-languages/scss/scss.contribution";
import "monaco-editor/esm/vs/basic-languages/html/html.contribution";
import "monaco-editor/esm/vs/basic-languages/xml/xml.contribution";
import "monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution";
import "monaco-editor/esm/vs/basic-languages/shell/shell.contribution";
import "monaco-editor/esm/vs/basic-languages/dockerfile/dockerfile.contribution";
import "monaco-editor/esm/vs/basic-languages/sql/sql.contribution";
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

// languageFor picks the highlighting for a file by its name.
const BY_EXTENSION: Record<string, string> = {
  go: "go",
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  json: "javascript",
  py: "python",
  rs: "rust",
  java: "java",
  kt: "kotlin",
  swift: "swift",
  c: "cpp",
  h: "cpp",
  cc: "cpp",
  cpp: "cpp",
  hpp: "cpp",
  cs: "csharp",
  rb: "ruby",
  php: "php",
  css: "css",
  scss: "scss",
  html: "html",
  htm: "html",
  xml: "xml",
  svg: "xml",
  yaml: "yaml",
  yml: "yaml",
  sh: "shell",
  bash: "shell",
  zsh: "shell",
  md: "markdown",
  toml: "ini",
  ini: "ini",
  sql: "sql",
};

export function languageFor(path: string): string {
  const name = path.split("/").pop() ?? "";
  if (name === "Dockerfile") return "dockerfile";
  if (name === "Makefile") return "shell";
  return BY_EXTENSION[name.split(".").pop()?.toLowerCase() ?? ""] ?? "plaintext";
}
