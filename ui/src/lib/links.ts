// Where a link written in a brief, a report or a message may go. Text comes
// from agents, so a link is let through only when it is one of the few kinds
// we know: the web, an in-page anchor, or a file of the task's repo. Anything
// else, javascript: above all, reads as plain text.

// LinkTo is a link as it is drawn: where it goes, whether in a new tab, and
// for refs a tooltip (title) and an accessible name (name).
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

// Refs are what a task's text can name besides the web and its files: the
// board's tasks, by id, and the task's own commits.
export interface Refs {
  titles: Map<string, string>; // every task the board knows: id to title
  commits: string[]; // the task's commit hashes, in full
}

// refTo is the link for a word that names a task the board knows, or one of
// the task's commits by 7 to 40 characters of its hash; null for any other
// word, which shows as written.
export function refTo(taskId: string, word: string, refs: Refs): LinkTo | null {
  const title = refs.titles.get(word);
  if (title !== undefined)
    return { href: `/tasks/${encodeURIComponent(word)}`, external: false, ...(title ? { title, name: `${word}, ${title}` } : {}) };
  if (!/^[0-9a-f]{7,40}$/i.test(word)) return null;
  const hash = refs.commits.find((commit) => commit.startsWith(word.toLowerCase()));
  if (!hash) return null;
  return {
    href: `/tasks/${encodeURIComponent(taskId)}/workspace?commit=${encodeURIComponent(hash)}`,
    external: true,
    name: `commit ${word} (opens in a new tab)`,
  };
}

// linkInto is where a link in a task's text goes: the web as it is, a file of
// the task into its Workspace preview, in a new tab, and with refs a task or
// commit named as the link's target; null when it isn't a link.
export function linkInto(taskId: string, href: string, refs?: Refs): LinkTo | null {
  const ref = refs && refTo(taskId, href.trim(), refs);
  if (ref) return ref;
  const safe = safeHref(href);
  if (!safe) return null;
  if (safe.kind !== "path") return { href: safe.href, external: safe.kind === "external" };
  const path = safe.href.split("#")[0];
  return { href: `/tasks/${encodeURIComponent(taskId)}/workspace?file=${encodeURIComponent(path)}&view=preview`, external: true };
}
