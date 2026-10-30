import { describe, expect, test } from "vitest";

// The task page's text must not pull Monaco into the page's bundle: only
// MonacoColorize, loaded with import() when code comes near the screen, may
// reach lib/monaco or monaco-editor. This follows each file's static imports
// all the way down; an import() is where a bundle splits, so it isn't followed.

const SOURCES: Record<string, string> = import.meta.glob(["/src/**/*.ts", "/src/**/*.tsx", "!/src/**/*.test.*"], {
  query: "?raw",
  import: "default",
  eager: true,
});

const KEPT_LIGHT = [
  "/src/components/molecules/CodeBox.tsx",
  "/src/components/molecules/LazyColorize.tsx",
  "/src/domains/tasks/EntryText.tsx",
  "/src/domains/tasks/ActivityFeed.tsx",
  "/src/domains/tasks/HandoffTimeline.tsx",
];

const STATIC_IMPORT = /^\s*(?:import(?!\s*\()|export)\b[^;'"]*?(?:from\s*)?["']([^"']+)["']/gm;

// monacoPath is the chain of imports from a file to Monaco, or null.
function monacoPath(file: string, seen = new Set<string>()): string[] | null {
  if (seen.has(file)) return null;
  seen.add(file);
  for (const [, spec] of SOURCES[file].matchAll(STATIC_IMPORT)) {
    if (spec === "monaco-editor" || spec.startsWith("monaco-editor/")) return [spec];
    const next = resolve(file, spec);
    if (!next) continue;
    if (next === "/src/lib/monaco.ts") return [next];
    const rest = monacoPath(next, seen);
    if (rest) return [next, ...rest];
  }
  return null;
}

// resolve finds the source file an import names, or null for a package.
function resolve(from: string, spec: string): string | null {
  const base = spec.startsWith("@/") ? `/src/${spec.slice(2)}` : spec.startsWith(".") ? new URL(spec, `file://${from}`).pathname : null;
  if (!base) return null;
  return ["", ".ts", ".tsx", "/index.ts", "/index.tsx"].map((ext) => base + ext).find((path) => path in SOURCES) ?? null;
}

describe("Monaco stays out of the task page's text", () => {
  test.each(KEPT_LIGHT)("%s imports nothing that reaches Monaco", (file) => {
    expect(SOURCES[file]).toBeDefined();
    expect(monacoPath(file)).toBeNull();
  });

  test("the check finds Monaco where it is", () => {
    expect(monacoPath("/src/components/molecules/MonacoColorize.tsx")).toEqual(["/src/lib/monaco.ts"]);
    expect(monacoPath("/src/components/molecules/MonacoView.tsx")).not.toBeNull();
  });
});
