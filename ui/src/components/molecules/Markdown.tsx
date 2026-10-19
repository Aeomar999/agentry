import { Fragment, MouseEvent, ReactNode } from "react";
import { ArrowUpRight, Square, SquareCheck } from "lucide-react";
import { cn } from "@/lib/cn";
import { LinkTo, safeHref } from "@/lib/links";

// The Markdown people and agents write, as text to read: headings, lists
// (nested, and task lists), tables, code blocks, quotes, rules, paragraphs;
// bold, italics, strikethrough, inline code, links, autolinks and images.
// Everything is built as React elements, never as HTML, so nothing in the
// text can run on the page.
//
// It reads two ways: in a chat, compact; as a document (doc), the way an
// editor's preview shows a README, with headings that can be linked to. A
// document's host draws its images and code blocks, so images can come from
// the workspace and code can be highlighted.
//
// A link goes where the host's link hook says, or, without one, where
// safeHref allows; anything else reads as its label. With a refs hook, a
// bare word or a one-word inline code can be a link too (a task id, a
// commit), drawn as a mono link with no code background.

export interface MarkdownProps {
  text: string;
  className?: string;
  doc?: boolean;
  image?: (src: string, alt: string) => ReactNode;
  code?: (code: string, language: string) => ReactNode;
  link?: (href: string) => LinkTo | null; // where a link goes; null: not a link
  refs?: (word: string) => LinkTo | null; // what a word names, if anything
  marks?: boolean; // links in the link colour, underlined, with ↗ on those that open a new tab
}

export interface Heading {
  level: number;
  text: string;
  id: string;
}

export function Markdown(props: MarkdownProps) {
  const { text, className, doc = false } = props;
  return (
    <div
      className={cn(
        "flex flex-col break-words text-ink-primary",
        doc ? "gap-4 text-[14px] leading-[1.7]" : "gap-2 text-[13px] leading-[1.6]",
        className,
      )}
    >
      {blocks(text, props, new Map())}
    </div>
  );
}

// headingsOf lists a document's headings, for its outline, with the same
// ids the rendered headings get.
export function headingsOf(text: string): Heading[] {
  const seen = new Map<string, number>();
  const out: Heading[] = [];
  let fenced = false;
  for (const line of text.replace(/\r/g, "").split("\n")) {
    if (line.trimStart().startsWith("```")) fenced = !fenced;
    const m = !fenced && line.match(/^(#{1,4})\s+(.*?)\s*#*$/);
    if (m) out.push({ level: m[1].length, text: plain(m[2]), id: slug(plain(m[2]), seen) });
  }
  return out;
}

const LIST = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;

function blocks(text: string, props: MarkdownProps, seen: Map<string, number>): ReactNode[] {
  const { doc = false } = props;
  const lines = text.replace(/\r/g, "").split("\n");
  const out: ReactNode[] = [];
  const key = () => out.length;
  for (let i = 0; i < lines.length;) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    // ```lang fenced code
    const fence = line.match(/^\s*```\s*([\w+#.-]*)/);
    if (fence) {
      const code: string[] = [];
      for (i++; i < lines.length && !lines[i].trimStart().startsWith("```"); i++) code.push(lines[i]);
      i++;
      const body = code.join("\n");
      out.push(
        props.code ? (
          <Fragment key={key()}>{props.code(body, fence[1].toLowerCase())}</Fragment>
        ) : (
          <pre
            key={key()}
            className="scrollbar-thin overflow-x-auto rounded-md border border-divider bg-surface-1 px-3 py-2 font-mono text-[12px] leading-5"
          >
            {body}
          </pre>
        ),
      );
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.*?)\s*#*$/);
    if (heading) {
      const level = heading[1].length;
      const id = slug(plain(heading[2]), seen);
      const size = doc
        ? [
            "",
            "text-[26px] font-bold border-b border-divider pb-2 mt-2",
            "text-[20px] font-semibold border-b border-divider pb-1.5 mt-3",
            "text-[16px] font-semibold mt-2",
            "text-[14px] font-semibold",
          ][level]
        : level <= 2
          ? "text-[14px] font-semibold"
          : "text-[13px] font-semibold";
      out.push(
        <p
          key={key()}
          id={doc ? id : undefined}
          role="heading"
          aria-level={level}
          className={cn("group scroll-mt-4 leading-snug text-ink-primary", size)}
        >
          {inline(heading[2], props)}
          {doc && (
            <a
              href={`#${id}`}
              aria-label="Link to this heading"
              className="ml-2 text-ink-faint opacity-0 group-hover:opacity-100 hover:text-info"
            >
              #
            </a>
          )}
        </p>,
      );
      i++;
      continue;
    }
    if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) {
      out.push(<hr key={key()} className="border-divider" />);
      i++;
      continue;
    }
    // A table: a header row, then a row of dashes.
    if (line.includes("|") && i + 1 < lines.length && /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(lines[i + 1])) {
      const align = cells(lines[i + 1]).map((c) => (c.startsWith(":") && c.endsWith(":") ? "center" : c.endsWith(":") ? "right" : "left"));
      const head = cells(line);
      const rows: string[][] = [];
      for (i += 2; i < lines.length && lines[i].includes("|") && lines[i].trim(); i++) rows.push(cells(lines[i]));
      out.push(
        <div key={key()} className="scrollbar-thin overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr>
                {head.map((c, n) => (
                  <th
                    key={n}
                    style={{ textAlign: align[n] as "left" }}
                    className="border border-divider bg-surface-1 px-3 py-1.5 font-semibold text-ink-primary"
                  >
                    {inline(c, props)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, r) => (
                <tr key={r} className="even:bg-surface-1/60">
                  {head.map((_, n) => (
                    <td
                      key={n}
                      style={{ textAlign: align[n] as "left" }}
                      className="border border-divider px-3 py-1.5 align-top text-ink-secondary"
                    >
                      {inline(row[n] ?? "", props)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>,
      );
      continue;
    }
    if (LIST.test(line)) {
      const items: string[] = [];
      for (; i < lines.length && (LIST.test(lines[i]) || (/^\s{2,}\S/.test(lines[i]) && items.length)); i++) items.push(lines[i]);
      out.push(<Fragment key={key()}>{list(items, props)}</Fragment>);
      continue;
    }
    if (line.startsWith(">")) {
      const quote: string[] = [];
      for (; i < lines.length && lines[i].startsWith(">"); i++) quote.push(lines[i].replace(/^>\s?/, ""));
      out.push(
        <blockquote key={key()} className="rounded-md bg-surface-1 px-4 py-2 text-ink-secondary">
          {blocks(quote.join("\n"), props, seen)}
        </blockquote>,
      );
      continue;
    }
    const paragraph: string[] = [];
    for (; i < lines.length && lines[i].trim() && !/^(#{1,4}\s|\s*```|>)/.test(lines[i]) && !LIST.test(lines[i]); i++)
      paragraph.push(lines[i].trim());
    out.push(
      <p key={key()} className={doc ? "text-ink-secondary" : undefined}>
        {inline(paragraph.join(" "), props)}
      </p>,
    );
  }
  return out;
}

// list draws list lines, nesting the ones indented under an item.
function list(lines: string[], props: MarkdownProps): ReactNode {
  const indentOf = (line: string) => line.match(/^\s*/)?.[0].replace(/\t/g, "  ").length ?? 0;
  const base = indentOf(lines[0]);
  const ordered = /^\s*\d/.test(lines[0]);
  const items: { text: string; children: string[] }[] = [];
  for (const line of lines) {
    const m = line.match(LIST);
    if (m && indentOf(line) <= base + 1) items.push({ text: m[3], children: [] });
    else if (items.length) {
      if (m) items[items.length - 1].children.push(line);
      else items[items.length - 1].text += " " + line.trim(); // a wrapped item
    }
  }
  const tasks = items.some((item) => /^\[[ xX]\]\s/.test(item.text));
  const List = ordered ? "ol" : "ul";
  return (
    <List
      className={cn(
        "flex flex-col gap-1",
        tasks ? "pl-1" : "pl-6",
        !tasks && (ordered ? "list-decimal" : "list-disc"),
        "marker:text-ink-faint",
      )}
    >
      {items.map((item, n) => {
        const task = item.text.match(/^\[([ xX])\]\s+(.*)$/);
        return (
          <li key={n} className={cn(task && "flex list-none items-start gap-2")}>
            {task &&
              (task[1] === " " ? (
                <Square className="mt-[5px] size-3.5 shrink-0 text-ink-faint" />
              ) : (
                <SquareCheck className="mt-[5px] size-3.5 shrink-0 text-success" />
              ))}
            <span className={cn(task && task[1] !== " " && "text-ink-muted")}>
              {inline(task ? task[2] : item.text, props)}
              {item.children.length > 0 && <div className="mt-1">{list(item.children, props)}</div>}
            </span>
          </li>
        );
      })}
    </List>
  );
}

function cells(row: string): string[] {
  return row
    .trim()
    .replace(/^\|/, "")
    .replace(/\|$/, "")
    .split("|")
    .map((c) => c.trim());
}

// `code`, **bold**, ~~struck~~, *italic* or _italic_, ![image](src), [link](href) and bare https:// links.
const INLINE =
  /(`[^`]+`|\*\*[^*]+\*\*|__[^_]+__|~~[^~]+~~|!\[[^\]]*\]\([^)\s]+(?:\s+"[^"]*")?\)|\[[^\]]+\]\([^)\s]+(?:\s+"[^"]*")?\)|\*[^*\s][^*]*\*|\b_[^_\s][^_]*_\b|https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g;

function inline(text: string, props: MarkdownProps): ReactNode {
  const parts = text.split(INLINE);
  return parts.map((part, i) => {
    if (i % 2 === 0) return <Fragment key={i}>{words(part, props)}</Fragment>;
    if (part.startsWith("`")) {
      const code = part.slice(1, -1);
      const to = props.refs && /^\S+$/.test(code) ? props.refs(code) : null;
      if (to) return <Fragment key={i}>{anchor(to, code, props, true)}</Fragment>;
      return (
        <code key={i} className="rounded-sm bg-surface-3 px-1 py-px font-mono text-[0.88em] text-ink-primary">
          {code}
        </code>
      );
    }
    if (part.startsWith("**") || part.startsWith("__"))
      return (
        <strong key={i} className="font-semibold text-ink-primary">
          {inline(part.slice(2, -2), props)}
        </strong>
      );
    if (part.startsWith("~~"))
      return (
        <del key={i} className="text-ink-muted">
          {part.slice(2, -2)}
        </del>
      );
    const image = part.match(/^!\[([^\]]*)\]\(([^)\s]+)/);
    if (image) return <Fragment key={i}>{props.image ? props.image(image[2], image[1]) : `[${image[1] || "image"}]`}</Fragment>;
    const link = part.match(/^\[([^\]]+)\]\(([^)\s]+)/);
    const bare = /^https?:\/\//.test(part) ? part : "";
    if (link || bare) {
      const href = link ? link[2] : bare;
      const label = link ? inline(link[1], { ...props, refs: undefined }) : bare; // no link inside a link
      const to = props.link ? props.link(href) : fallback(href);
      // A hook's href is the host's own, but it can't send a link anywhere safeHref refuses.
      if (!to || !safeHref(to.href)) return <Fragment key={i}>{label}</Fragment>;
      return <Fragment key={i}>{anchor(to, label, props)}</Fragment>;
    }
    return <em key={i}>{inline(part.slice(1, -1), props)}</em>;
  });
}

// Without a link hook, a link goes where safeHref allows, to the href it returns.
function fallback(href: string): LinkTo | null {
  const safe = safeHref(href);
  return safe && { href: safe.href, external: safe.kind === "external" };
}

// words makes the words of plain text that the refs hook knows into links.
function words(text: string, props: MarkdownProps): ReactNode {
  if (!props.refs) return text;
  const out: ReactNode[] = [];
  let from = 0;
  for (const word of text.matchAll(/[\w-]+/g)) {
    const to = props.refs(word[0]);
    if (!to) continue;
    out.push(text.slice(from, word.index), <Fragment key={word.index}>{anchor(to, word[0], props, true)}</Fragment>);
    from = word.index + word[0].length;
  }
  if (!out.length) return text;
  out.push(text.slice(from));
  return out;
}

function anchor(to: LinkTo, label: ReactNode, props: MarkdownProps, mono = false): ReactNode {
  const marked = props.marks && to.external;
  return (
    <a
      href={to.href}
      onClick={
        to.onClick
          ? (event) => {
              if (!plainClick(event)) return; // a new tab or window, as the browser does
              event.preventDefault();
              to.onClick!();
            }
          : undefined
      }
      target={to.external ? "_blank" : undefined}
      rel={to.external ? "noopener noreferrer" : undefined}
      title={to.title}
      aria-label={to.name}
      className={cn(
        props.marks ? "text-link underline underline-offset-2" : "text-info underline-offset-2 hover:underline",
        mono && "font-mono text-[0.88em]",
      )}
    >
      {label}
      {marked && " "}
      {marked && <ArrowUpRight aria-hidden className="inline size-3 align-[-1px]" />}
      {marked && !to.name && <span className="sr-only">(opens in a new tab)</span>}
    </a>
  );
}

// plainClick says whether a click is a plain left click, with no key that
// asks the browser for a new tab, a new window or a download.
function plainClick(event: MouseEvent): boolean {
  return event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey;
}

function plain(text: string): string {
  return text
    .replace(/[`*_~]/g, "")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .trim();
}

function slug(text: string, seen: Map<string, number>): string {
  const base =
    text
      .toLowerCase()
      .replace(/[^\p{L}\p{N}\s-]/gu, "")
      .trim()
      .replace(/\s+/g, "-") || "section";
  const n = seen.get(base) ?? 0;
  seen.set(base, n + 1);
  return n ? `${base}-${n}` : base;
}
