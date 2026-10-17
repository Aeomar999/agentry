import { MouseEvent as ReactMouseEvent, useEffect, useMemo, useRef, useState, WheelEvent } from "react";
import { Maximize, Minus, Plus, Scan } from "lucide-react";
import { CodeFence } from "@/components/molecules/CodeFence";
import { headingsOf, Markdown } from "@/components/molecules/Markdown";
import { useResource } from "@/live/use-resource";
import { cn } from "@/lib/cn";

// A workspace file as it reads, not as it is written, the way an editor's
// preview shows it: Markdown as a document, with its images from the
// workspace, highlighted code and an outline; an SVG as the drawing, to
// zoom and pan. previewable says which files have one.

export const previewable = (path: string) => /\.(md|markdown|mdx|svg)$/i.test(path);

interface PreviewProps {
  taskId: string;
  path: string;
  commit: string; // "" for now
  text: string;
  onOpen: (path: string) => void; // a link to another file of the workspace
  outline?: boolean; // the document's outline beside it, when there is room: not next to the code
}

export function WorkspacePreview(props: PreviewProps) {
  return /\.svg$/i.test(props.path) ? <SvgPreview src={svgSource(props.text)} name={props.path} /> : <MarkdownPreview {...props} />;
}

function svgSource(text: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}`;
}

// ---- Markdown -------------------------------------------------------------

function MarkdownPreview({ taskId, path, commit, text, onOpen, outline = true }: PreviewProps) {
  const headings = useMemo(() => headingsOf(text).filter((h) => h.level <= 3), [text]);
  const scroller = useRef<HTMLDivElement>(null);
  const [current, setCurrent] = useState("");

  // The outline follows what you read: the last heading above the top third.
  useEffect(() => {
    const box = scroller.current;
    if (!box || headings.length === 0) return;
    const onScroll = () => {
      const line = box.getBoundingClientRect().top + box.clientHeight / 3;
      let at = headings[0].id;
      for (const h of headings) {
        const el = document.getElementById(h.id);
        if (el && el.getBoundingClientRect().top <= line) at = h.id;
      }
      setCurrent(at);
    };
    onScroll();
    box.addEventListener("scroll", onScroll, { passive: true });
    return () => box.removeEventListener("scroll", onScroll);
  }, [headings]);

  const folder = path.includes("/") ? path.slice(0, path.lastIndexOf("/") + 1) : "";
  return (
    <div className="flex h-full min-h-0">
      <div ref={scroller} className="min-w-0 flex-1 scrollbar-thin overflow-y-auto">
        <article className="mx-auto max-w-[880px] px-10 py-8">
          <Markdown
            doc
            text={text}
            code={(code, language) => <CodeFence code={code} language={language} />}
            image={(src, alt) => <WorkspaceImage taskId={taskId} commit={commit} src={src} folder={folder} alt={alt} />}
            link={(href) => {
              if (/^(https?:|mailto:)/i.test(href)) return { href, external: true };
              if (href.startsWith("#"))
                return {
                  href,
                  external: false,
                  onClick: () => document.getElementById(href.slice(1))?.scrollIntoView({ behavior: "smooth" }),
                };
              const file = resolve(folder, href.split("#")[0]);
              return { href: "#", external: false, onClick: () => onOpen(file) };
            }}
          />
        </article>
      </div>
      {outline && headings.length > 2 && (
        <nav
          aria-label="Outline"
          className="hidden w-60 shrink-0 scrollbar-thin overflow-y-auto border-l border-divider py-6 pr-3 pl-4 xl:block"
        >
          <span className="mb-2 block text-[11px] font-semibold tracking-wider text-ink-faint uppercase">Outline</span>
          {headings.map((h) => (
            <button
              key={h.id}
              type="button"
              onClick={() => document.getElementById(h.id)?.scrollIntoView({ behavior: "smooth" })}
              style={{ paddingLeft: (h.level - 1) * 12 }}
              className={cn(
                "block w-full cursor-pointer truncate py-1 text-left text-[12px] hover:text-ink-primary",
                h.id === current ? "font-medium text-ink-primary" : "text-ink-muted",
              )}
            >
              {h.text}
            </button>
          ))}
        </nav>
      )}
    </div>
  );
}

// resolve is a link's path from the folder of the file it is in.
function resolve(folder: string, target: string): string {
  const parts = (target.startsWith("/") ? target.slice(1) : folder + target).split("/");
  const out: string[] = [];
  for (const part of parts) {
    if (part === "..") out.pop();
    else if (part && part !== ".") out.push(part);
  }
  return out.join("/");
}

interface Contents {
  now: string;
  binary: boolean;
  data?: string;
  tooBig: boolean;
}

const MIME: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  svg: "image/svg+xml",
};

// An image a document names: from the web as it is, or from the workspace,
// read as the file it is, at the same commit.
function WorkspaceImage({
  taskId,
  commit,
  src,
  folder,
  alt,
}: {
  taskId: string;
  commit: string;
  src: string;
  folder: string;
  alt: string;
}) {
  const external = /^(https?:|data:)/i.test(src);
  const path = external ? "" : resolve(folder, src.split(/[?#]/)[0]);
  const file = useResource<Contents>(
    path ? `/api/tasks/${encodeURIComponent(taskId)}/file?path=${encodeURIComponent(path)}${commit ? `&commit=${commit}` : ""}` : null,
  ).data;
  if (external) return <img src={src} alt={alt} className="inline-block max-w-full rounded-sm" />;
  if (!file) return <span className="inline-block h-24 w-full max-w-md animate-pulse rounded-md bg-surface-2" aria-label={alt} />;
  const kind = MIME[path.split(".").pop()?.toLowerCase() ?? ""];
  const url = /\.svg$/i.test(path) && !file.binary ? svgSource(file.now) : file.data && kind ? `data:${kind};base64,${file.data}` : "";
  if (!url) return <span className="text-[12px] text-ink-muted">[{alt || path}: can't show it here]</span>;
  return <img src={url} alt={alt} title={path} className="inline-block max-w-full rounded-sm" />;
}

// ---- SVG ------------------------------------------------------------------

type Backdrop = "dark" | "light" | "checker";

// A drawing to look at closely: fit to the window at first, zoom with the
// buttons, a pinch or Ctrl and the wheel, drag to pan, on a backdrop of
// your choice.
export function SvgPreview({ src, name }: { src: string; name: string }) {
  const box = useRef<HTMLDivElement>(null);
  const [natural, setNatural] = useState({ w: 0, h: 0 });
  const [zoom, setZoom] = useState<number | "fit">("fit");
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [backdrop, setBackdrop] = useState<Backdrop>("checker");
  const drag = useRef<{ x: number; y: number; ox: number; oy: number } | null>(null);

  const fit = () => {
    const el = box.current;
    if (!el || !natural.w || !natural.h) return 1;
    return Math.min((el.clientWidth - 64) / natural.w, (el.clientHeight - 64) / natural.h, 4);
  };
  const scale = zoom === "fit" ? fit() : zoom;
  const setScale = (next: number) => setZoom(Math.min(16, Math.max(0.05, next)));
  useEffect(() => {
    setZoom("fit");
    setOffset({ x: 0, y: 0 });
  }, [src]);

  // A pinch, or Ctrl and the wheel, zooms the drawing, in step with the
  // gesture; the app itself never zooms on it (main.tsx).
  function onWheel(event: WheelEvent<HTMLDivElement>) {
    if (!event.ctrlKey && !event.metaKey) return;
    setScale(scale * Math.exp(-event.deltaY * 0.01));
  }
  function onDown(event: ReactMouseEvent<HTMLDivElement>) {
    drag.current = { x: event.clientX, y: event.clientY, ox: offset.x, oy: offset.y };
  }
  function onMove(event: ReactMouseEvent<HTMLDivElement>) {
    if (drag.current)
      setOffset({ x: drag.current.ox + event.clientX - drag.current.x, y: drag.current.oy + event.clientY - drag.current.y });
  }

  const backdrops: Record<Backdrop, string> = {
    dark: "bg-[#16181d]",
    light: "bg-white",
    checker:
      "bg-[length:20px_20px] bg-[conic-gradient(var(--color-surface-2)_25%,var(--color-surface-0)_0_50%,var(--color-surface-2)_0_75%,var(--color-surface-0)_0)]",
  };
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-divider px-2 text-[12px]">
        <Tool label="Zoom out" onClick={() => setScale(scale / 1.25)}>
          <Minus />
        </Tool>
        <span className="w-12 text-center font-mono text-[11px] text-ink-muted">{Math.round(scale * 100)}%</span>
        <Tool label="Zoom in" onClick={() => setScale(scale * 1.25)}>
          <Plus />
        </Tool>
        <Tool label="Fit to the window" onClick={() => (setZoom("fit"), setOffset({ x: 0, y: 0 }))} active={zoom === "fit"}>
          <Maximize />
        </Tool>
        <Tool label="Actual size" onClick={() => (setZoom(1), setOffset({ x: 0, y: 0 }))} active={zoom === 1}>
          <Scan />
        </Tool>
        {natural.w > 0 && (
          <span className="ml-2 font-mono text-[11px] text-ink-faint">
            {natural.w} × {natural.h}
          </span>
        )}
        <span className="ml-auto flex rounded-md border border-divider p-0.5" role="radiogroup" aria-label="Backdrop">
          {(["dark", "light", "checker"] as Backdrop[]).map((b) => (
            <button
              key={b}
              type="button"
              role="radio"
              aria-checked={backdrop === b}
              onClick={() => setBackdrop(b)}
              className={cn(
                "h-6 cursor-pointer rounded-sm px-2 text-[11px] capitalize",
                backdrop === b ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:text-ink-primary",
              )}
            >
              {b}
            </button>
          ))}
        </span>
      </div>
      <div
        ref={box}
        onWheel={onWheel}
        onMouseDown={onDown}
        onMouseMove={onMove}
        onMouseUp={() => (drag.current = null)}
        onMouseLeave={() => (drag.current = null)}
        onDoubleClick={() => (setZoom("fit"), setOffset({ x: 0, y: 0 }))}
        className={cn("relative min-h-0 flex-1 cursor-grab overflow-hidden select-none active:cursor-grabbing", backdrops[backdrop])}
      >
        <img
          src={src}
          alt={name}
          draggable={false}
          onLoad={(event) => setNatural({ w: event.currentTarget.naturalWidth || 800, h: event.currentTarget.naturalHeight || 600 })}
          className="absolute top-1/2 left-1/2 max-w-none"
          style={{
            width: natural.w ? natural.w * scale : undefined,
            transform: `translate(calc(-50% + ${offset.x}px), calc(-50% + ${offset.y}px))`,
          }}
        />
      </div>
    </div>
  );
}

function Tool({
  label,
  onClick,
  active = false,
  children,
}: {
  label: string;
  onClick: () => void;
  active?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className={cn(
        "inline-flex size-7 cursor-pointer items-center justify-center rounded-sm text-ink-muted hover:bg-surface-2 hover:text-ink-primary [&>svg]:size-3.5",
        active && "bg-surface-3 text-ink-primary",
      )}
    >
      {children}
    </button>
  );
}
