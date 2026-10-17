// The sandbox an agent's page runs in, such as a question's page. It
// is shown in an iframe with its own origin (sandbox="allow-scripts", never
// allow-same-origin), so it can't read Agentry's token or pages, and a
// content policy that lets nothing load from the network. It gets Agentry's
// colours as CSS variables and plain styles for plain HTML, so it looks like
// the rest of the app in either theme, and the one script its host gives it.

export const POLICY = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; script-src 'unsafe-inline'";

// What a page can use, by the names agents are told (coordinator.PageDesign).
const VARIABLES: Record<string, string> = {
  "--bg": "--color-surface-0",
  "--panel": "--color-surface-1",
  "--raised": "--color-surface-2",
  "--border": "--color-divider",
  "--text": "--color-ink-primary",
  "--text-2": "--color-ink-secondary",
  "--muted": "--color-ink-muted",
  "--accent": "--color-info",
  "--good": "--color-success",
  "--warn": "--color-warning",
  "--bad": "--color-danger",
  "--brand": "--color-brand",
  "--mono": "--font-mono",
  "--sans": "--font-sans",
};

export function pageDocument(page: string, script: string): string {
  const root = getComputedStyle(document.documentElement);
  const variables = Object.entries(VARIABLES)
    .map(([name, token]) => `${name}: ${root.getPropertyValue(token).trim()};`)
    .join(" ");
  return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="${POLICY}">
<style>
  :root { ${variables} color-scheme: ${document.documentElement.classList.contains("light") ? "light" : "dark"}; }
  * { box-sizing: border-box; }
  body { margin: 0; padding: 24px; font: 14px/1.55 var(--sans); color: var(--text); background: var(--bg); }
  h1, h2, h3 { margin: 0 0 8px; line-height: 1.25; } h1 { font-size: 20px; } h2 { font-size: 16px; } h3 { font-size: 14px; }
  p { margin: 0 0 12px; color: var(--text-2); }
  code, pre { font-family: var(--mono); font-size: 12px; } pre { background: var(--panel); border: 1px solid var(--border); padding: 12px; overflow: auto; }
  table { border-collapse: collapse; width: 100%; } th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  th { color: var(--muted); font-weight: 600; font-size: 12px; }
  button { font: inherit; font-weight: 500; color: var(--text); background: var(--raised); border: 1px solid var(--border); border-radius: 3px; padding: 6px 12px; cursor: pointer; }
  button:hover { border-color: var(--muted); }
  svg { max-width: 100%; height: auto; }
</style>
<script>
  // A pinch never zooms the app, from inside a page either (main.tsx).
  window.addEventListener("wheel", function (e) { if (e.ctrlKey) e.preventDefault(); }, { passive: false });
  ["gesturestart", "gesturechange"].forEach(function (g) { window.addEventListener(g, function (e) { e.preventDefault(); }); });
${script}</script>
</head><body>${page}</body></html>`;
}
