import { Fragment, ReactNode } from "react";
import { cn } from "@/lib/cn";

// The Markdown agents write, as text people read: headings, lists, code
// blocks, quotes and paragraphs; bold, italics, inline code and links.
// Everything is built as React elements, never as HTML, so nothing an agent
// writes can run on the page.

export function Markdown({ text, className }: { text: string; className?: string }) {
  return <div className={cn("flex flex-col gap-2 text-[13px] leading-[1.6] break-words text-ink-primary", className)}>{blocks(text)}</div>;
}

function blocks(text: string): ReactNode[] {
  const lines = text.replace(/\r/g, "").split("\n");
  const out: ReactNode[] = [];
  for (let i = 0; i < lines.length;) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    if (line.trimStart().startsWith("```")) {
      const code: string[] = [];
      for (i++; i < lines.length && !lines[i].trimStart().startsWith("```"); i++) code.push(lines[i]);
      i++;
      out.push(
        <pre
          key={out.length}
          className="scrollbar-thin overflow-x-auto rounded-md border border-divider bg-surface-1 px-3 py-2 font-mono text-[12px] leading-5"
        >
          {code.join("\n")}
        </pre>,
      );
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.*)$/);
    if (heading) {
      out.push(
        <p key={out.length} className={cn("font-semibold text-ink-primary", heading[1].length <= 2 ? "text-[14px]" : "text-[13px]")}>
          {inline(heading[2])}
        </p>,
      );
      i++;
      continue;
    }
    if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
      const ordered = /^\s*\d/.test(line);
      const items: string[] = [];
      for (; i < lines.length && /^\s*([-*+]|\d+[.)])\s+/.test(lines[i]); i++) {
        items.push(lines[i].replace(/^\s*([-*+]|\d+[.)])\s+/, ""));
        // A wrapped item goes on, indented, on the lines under it.
        while (i + 1 < lines.length && /^\s{2,}\S/.test(lines[i + 1]) && !/^\s*([-*+]|\d+[.)])\s+/.test(lines[i + 1])) {
          items[items.length - 1] += " " + lines[++i].trim();
        }
      }
      const List = ordered ? "ol" : "ul";
      out.push(
        <List key={out.length} className={cn("flex flex-col gap-1 pl-5", ordered ? "list-decimal" : "list-disc", "marker:text-ink-faint")}>
          {items.map((item, n) => (
            <li key={n}>{inline(item.replace(/^\[([ xX])\]\s+/, (_, done) => (done === " " ? "☐ " : "☑ ")))}</li>
          ))}
        </List>,
      );
      continue;
    }
    if (line.startsWith(">")) {
      const quote: string[] = [];
      for (; i < lines.length && lines[i].startsWith(">"); i++) quote.push(lines[i].replace(/^>\s?/, ""));
      out.push(
        <blockquote key={out.length} className="border-l-2 border-divider-2 pl-3 text-ink-secondary">
          {inline(quote.join(" "))}
        </blockquote>,
      );
      continue;
    }
    const paragraph: string[] = [];
    for (; i < lines.length && lines[i].trim() && !/^(#{1,4}\s|```|>|\s*([-*+]|\d+[.)])\s+)/.test(lines[i]); i++)
      paragraph.push(lines[i].trim());
    out.push(<p key={out.length}>{inline(paragraph.join(" "))}</p>);
  }
  return out;
}

// **bold**, *italic* or _italic_, `code` and [text](https://link).
const INLINE = /(`[^`]+`|\*\*[^*]+\*\*|__[^_]+__|\*[^*\s][^*]*\*|_[^_\s][^_]*_|\[[^\]]+\]\([^)\s]+\))/g;

function inline(text: string): ReactNode {
  const parts = text.split(INLINE);
  return parts.map((part, i) => {
    if (i % 2 === 0) return <Fragment key={i}>{part}</Fragment>;
    if (part.startsWith("`"))
      return (
        <code key={i} className="rounded-sm bg-surface-3 px-1 py-px font-mono text-[12px] text-ink-primary">
          {part.slice(1, -1)}
        </code>
      );
    if (part.startsWith("**") || part.startsWith("__"))
      return (
        <strong key={i} className="font-semibold">
          {part.slice(2, -2)}
        </strong>
      );
    const link = part.match(/^\[([^\]]+)\]\(([^)\s]+)\)$/);
    if (link) {
      const safe = /^https?:\/\//.test(link[2]);
      return safe ? (
        <a key={i} href={link[2]} target="_blank" rel="noreferrer noopener" className="text-info underline-offset-2 hover:underline">
          {link[1]}
        </a>
      ) : (
        <Fragment key={i}>{link[1]}</Fragment>
      );
    }
    return <em key={i}>{part.slice(1, -1)}</em>;
  });
}
