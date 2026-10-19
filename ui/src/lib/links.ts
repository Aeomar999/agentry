// Where a link written in a brief, a report or a message may go. Text comes
// from agents, so a link is let through only when it is one of the few kinds
// we know: the web, an in-page anchor, or a file of the task's repo. Anything
// else, javascript: above all, reads as plain text.

// LinkTo is a link as it is drawn: where it goes, whether in a new tab, and
// maybe a tooltip (title) and an accessible name (name).
export interface LinkTo {
  href: string;
  external: boolean; // opens in a new tab
  onClick?: () => void; // goes there itself, instead of the browser
  title?: string;
  name?: string;
}

export type SafeHref =
  | { kind: "external"; href: string } // the web, in a new tab
  | { kind: "anchor"; href: string } // #heading, on this page
  | { kind: "path"; href: string }; // a file of the repo, from its root

const SCHEME = /^([A-Za-z][A-Za-z0-9+.-]*):/;
const WEB = ["http:", "https:", "mailto:"];

// safeHref checks a link as written and returns the href to render, or null
// when it isn't a link. Callers render the href it returns, never the raw
// text: browsers read `/\host` as another site, and `java\tscript:` as
// javascript:.
export function safeHref(raw: string): SafeHref | null {
  // Browsers drop control characters anywhere in a URL, so we do too.
  const text = raw.replace(/[\u0000-\u001F\u007F]/g, "").trim();
  if (/^[/\\]{2}/.test(text)) return external(`https:${text}`); // //host, \\host or any mix: another site
  const scheme = text.match(SCHEME);
  if (scheme) return WEB.includes(`${scheme[1].toLowerCase()}:`) ? external(text) : null;
  if (text.startsWith("#")) return { kind: "anchor", href: text };
  const path = repoPath(text);
  return path ? { kind: "path", href: path } : null;
}

function external(text: string): SafeHref | null {
  try {
    const url = new URL(text);
    return WEB.includes(url.protocol) ? { kind: "external", href: url.href } : null;
  } catch {
    return null;
  }
}

// repoPath is a path from the repo's root: a leading / or ./ dropped, and
// . and .. resolved, as a browser would (%2e counts as a dot). A path that
// climbs above the root isn't a link. A ?query or #anchor stays as written.
function repoPath(text: string): string {
  const [, path, rest] = text.match(/^([^?#]*)(.*)$/)!;
  const segments: string[] = [];
  for (const segment of path.split(/[/\\]/)) {
    const dots = segment.replace(/%2e/gi, ".");
    if (dots === "..") {
      if (!segments.length) return "";
      segments.pop();
    } else if (segment && dots !== ".") segments.push(segment);
  }
  return segments.length ? segments.join("/") + rest : "";
}
