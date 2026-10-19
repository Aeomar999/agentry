import { KeyboardEvent, useLayoutEffect, useRef, useState } from "react";
import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  ChevronRight,
  CircleCheck,
  FileText,
  LayoutTemplate,
  MessageCircleQuestionMark,
  Sparkles,
  TriangleAlert,
} from "lucide-react";
import { Avatar } from "@/components/molecules/Avatar";
import { Markdown } from "@/components/molecules/Markdown";
import { Mono } from "@/components/molecules/Mono";
import { api } from "@/api/client";
import type { TaskDetail } from "@/api/types";
import { CHAT_TYPES, ChatEntry, ChatItem, chatOf, threadOf } from "@/lib/chat";
import { clockTime } from "@/lib/time";
import { cn } from "@/lib/cn";
import { linkInto } from "@/lib/links";
import { useEvents } from "@/live/use-events";
import { useToast } from "./Toasts";
import { useOpenQuestion } from "@/components/templates/AdminShell";

// Talk to the task's coordinator while the task goes on. What you send
// stops the turn in progress, so the coordinator reads it at once and
// decides again: usually it hands the work straight back, with its plan,
// changed for what you said, and says in a line what it changed.
//
// Laid out like any chat: newest at the bottom, each speaker's run under one
// avatar, a line for each day, and the agents' work between two messages
// folded into one line that opens on a click.

const SUGGESTIONS = ["Where are we?", "What's left before it's done?", "Wrap up what's done and finish."];

interface CoordinatorChatProps {
  task: TaskDetail;
  onSent: () => void;
  onOpenQuestion?: (questionId: string) => void; // where its question is answered; the shell's dock by default
}

export function CoordinatorChat({ task, onSent, onOpenQuestion }: CoordinatorChatProps) {
  const notify = useToast();
  const shellQuestion = useOpenQuestion();
  const openQuestion = onOpenQuestion ?? shellQuestion;
  // Only the events the chat reads, every one of them: a long task's tool
  // calls and plans stay on the server.
  const chat = useEvents({ task: task.id, types: CHAT_TYPES });
  const entries = chat.loading ? [] : threadOf(chatOf(task, chat.events));
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState("");
  const field = useRef<HTMLTextAreaElement>(null);

  const open = task.turnList?.find((t) => !t.ended);
  const thinking = open?.agent === task.coordinator;
  const working = open && !thinking ? open.agent : "";
  const asked = task.status === "needs you" && task.hasQuestion;
  const question = [...entries].reverse().find((e) => e.type === "message" && e.item.kind === "question");
  const lastQuestion = question?.key;
  const lastAsker = question?.type === "message" ? question.item.who : "";

  // Stay at the bottom, where the newest message is, while you are there.
  const scroller = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const [away, setAway] = useState(false);
  useLayoutEffect(() => {
    const box = scroller.current;
    if (box && atBottom.current) box.scrollTop = box.scrollHeight;
  }, [entries.length, sending, thinking]);
  const onScroll = () => {
    const box = scroller.current!;
    atBottom.current = box.scrollHeight - box.scrollTop - box.clientHeight < 80;
    setAway(!atBottom.current);
  };
  const toBottom = () => scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });

  async function send(text = draft) {
    const message = text.trim();
    if (!message || sending) return;
    setSending(message);
    setDraft("");
    atBottom.current = true;
    try {
      await api("POST", `/api/tasks/${encodeURIComponent(task.id)}/message`, { text: message });
      onSent();
    } catch (failure) {
      setDraft(message);
      notify("danger", "The message wasn't sent", (failure as Error).message);
    } finally {
      setSending("");
    }
  }
  function onKey(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      send();
    }
  }
  function suggest(text: string) {
    setDraft(text);
    field.current?.focus();
  }

  const status =
    task.status === "done"
      ? { text: "Finished the task", tone: "text-success", pulse: false }
      : asked
        ? { text: "Waiting for your answer", tone: "text-warning", pulse: true }
        : thinking
          ? { text: "Thinking…", tone: "text-success", pulse: true }
          : working
            ? { text: `${working} is working on it`, tone: "text-ink-muted", pulse: true }
            : { text: "Between turns", tone: "text-ink-muted", pulse: false };
  const hint =
    task.status === "done"
      ? "Sending opens the task again."
      : working
        ? `Sending pauses ${working} so the coordinator reads it now; ${working}'s work and plan carry on after.`
        : thinking
          ? "Sending makes the coordinator decide again with what you said."
          : "The coordinator reads this at once.";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b border-divider px-6">
        <Avatar name={task.coordinator} />
        <div className="flex min-w-0 flex-col leading-tight">
          <span className="font-mono text-[13px] font-semibold text-ink-primary">{task.coordinator}</span>
          <span className={cn("flex items-center gap-1.5 text-[11px]", status.tone)}>
            {status.pulse && <span className="size-1.5 animate-pulse rounded-full bg-current" aria-hidden />}
            {status.text}
          </span>
        </div>
        <span className="ml-auto text-[11px] text-ink-faint">Runs task {task.id}</span>
      </header>

      <div className="relative min-h-0 flex-1">
        {/* Positioned, so screen-reader-only text in a message is placed in it, not the page. */}
        <div ref={scroller} onScroll={onScroll} className="relative h-full scrollbar-thin overflow-y-auto">
          <ol className="mx-auto flex max-w-3xl flex-col px-6 pt-4 pb-6">
            {chat.loading && !chat.error && <li className="py-6 text-center text-[12px] text-ink-faint">Loading the conversation…</li>}
            {chat.error && <li className="py-6 text-center text-[12px] text-warning">Couldn't read the conversation: {chat.error}</li>}
            {entries.map((entry) => (
              <Entry
                key={entry.key}
                entry={entry}
                answering={asked && entry.key === lastQuestion}
                hasPage={Boolean(task.page)}
                taskId={task.id}
                onOpenQuestion={() => openQuestion(`${task.id}/question/open`)}
              />
            ))}
            {sending && <Mine text={sending} at="" opensRun pending />}
            {thinking && <Typing name={task.coordinator} />}
          </ol>
        </div>
        {away && (
          <button
            type="button"
            onClick={toBottom}
            aria-label="Newest messages"
            className="absolute bottom-3 left-1/2 inline-flex size-8 -translate-x-1/2 cursor-pointer items-center justify-center rounded-md border border-divider-2 bg-surface-2 text-ink-primary hover:bg-surface-3 [&>svg]:size-4"
          >
            <ArrowDown />
          </button>
        )}
      </div>

      <div className="shrink-0 px-6 pt-1 pb-4">
        <div className="mx-auto flex max-w-3xl flex-col gap-2">
          {!draft && !asked && task.status !== "done" && (
            <div className="flex flex-wrap gap-1.5">
              {SUGGESTIONS.map((text) => (
                <button
                  key={text}
                  type="button"
                  onClick={() => suggest(text)}
                  className="h-7 cursor-pointer rounded-md border border-divider px-3 text-[12px] text-ink-secondary hover:border-divider-2 hover:bg-surface-2 hover:text-ink-primary"
                >
                  {text}
                </button>
              ))}
            </div>
          )}
          {asked ? (
            <button
              type="button"
              onClick={() => openQuestion(`${task.id}/question/open`)}
              className="flex h-14 cursor-pointer items-center gap-3 rounded-lg border border-warning/50 bg-surface-1 px-4 text-left text-[13px] text-ink-primary transition-colors duration-(--dur-fast) hover:border-warning hover:bg-surface-2"
            >
              <MessageCircleQuestionMark className="size-4 shrink-0 text-warning" />
              <span className="min-w-0 flex-1">
                <span className="font-medium">
                  {lastAsker === task.coordinator ? "The coordinator" : "Agentry"} is waiting for your answer.
                </span>{" "}
                <span className="text-ink-muted">Answer it in Questions{task.page ? ", with the page it made" : ""}.</span>
              </span>
              <span className="inline-flex shrink-0 items-center gap-1 text-[12px] font-medium text-warning">
                Open <ChevronRight className="size-4" />
              </span>
            </button>
          ) : (
            <div className="flex flex-col rounded-lg border border-divider-2 bg-surface-1 transition-colors duration-(--dur-fast) focus-within:border-focus">
              <textarea
                ref={field}
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={onKey}
                rows={Math.min(8, Math.max(2, draft.split("\n").length))}
                placeholder={`Message ${task.coordinator}…`}
                aria-label={`Message ${task.coordinator}`}
                className="resize-none bg-transparent px-3.5 pt-3 text-[13px] leading-5 text-ink-primary outline-none placeholder:text-ink-faint"
              />
              <div className="flex items-center gap-3 px-3.5 pt-1 pb-2.5">
                <span className="min-w-0 flex-1 text-[11px] text-ink-faint">{hint}</span>
                <span className="hidden text-[11px] text-ink-faint sm:inline">Shift+Enter for a new line</span>
                <button
                  type="button"
                  onClick={() => send()}
                  disabled={!draft.trim() || Boolean(sending)}
                  aria-label="Send"
                  className="inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-md bg-brand text-white transition-colors duration-(--dur-fast) hover:bg-brand-hover disabled:cursor-default disabled:bg-surface-3 disabled:text-ink-faint [&>svg]:size-4"
                >
                  <ArrowUp />
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

interface QuestionProps {
  answering: boolean; // the question still waiting for you
  hasPage: boolean; // it came with a page the coordinator made
  onOpenQuestion: () => void;
  taskId: string; // for the address of a file its messages link to
}

function Entry({ entry, ...question }: { entry: ChatEntry } & QuestionProps) {
  if (entry.type === "day") {
    return (
      <li className="flex items-center gap-3 py-4 text-[11px] font-medium text-ink-faint" aria-label={entry.label}>
        <span className="h-px flex-1 bg-divider" />
        {entry.label}
        <span className="h-px flex-1 bg-divider" />
      </li>
    );
  }
  if (entry.type === "work") return <Work items={entry.items} taskId={question.taskId} />;
  const { item, opensRun } = entry;
  if (item.kind === "you")
    return <Mine text={item.text} at={item.at} opensRun={opensRun} viaClaude={item.viaClaude} viaArtifact={item.viaArtifact} />;
  return <Theirs item={item} opensRun={opensRun} {...question} />;
}

// What you said: a filled bubble on the right.
// Sent through Claude in the Workspace, it says so under it.
function Mine({
  text,
  at,
  opensRun,
  pending = false,
  viaClaude = false,
  viaArtifact = false,
}: {
  text: string;
  at: string;
  opensRun: boolean;
  pending?: boolean;
  viaClaude?: boolean;
  viaArtifact?: boolean;
}) {
  return (
    <li className={cn("group flex flex-col items-end", opensRun ? "mt-5" : "mt-1", pending && "opacity-60")}>
      {viaClaude && (
        <span className="mb-1 inline-flex items-center gap-1 text-[11px] text-ink-muted [&>svg]:size-3">
          <Sparkles /> sent from Claude in the Workspace
        </span>
      )}
      {viaArtifact && (
        <span className="mb-1 inline-flex items-center gap-1 text-[11px] text-ink-muted [&>svg]:size-3">
          <LayoutTemplate /> from an artifact
        </span>
      )}
      <div className="max-w-[80%] rounded-lg rounded-br-sm bg-surface-3 px-3.5 py-2 text-[13px] leading-[1.6] break-words whitespace-pre-wrap text-ink-primary">
        {text}
      </div>
      <Mono className="mt-1 h-4 text-[11px] text-ink-faint opacity-0 transition-opacity group-hover:opacity-100">
        {pending ? "Sending…" : clockTime(at)}
      </Mono>
    </li>
  );
}

// What the coordinator said: its avatar and name once per run, then plain
// text, as assistants read in a chat. A question or the task's summary is a
// card. The question waiting for you is answered in one place, the dock's
// Questions, with its page and choices: the card opens it there.
function Theirs({ item, opensRun, answering, hasPage, onOpenQuestion, taskId }: { item: ChatItem; opensRun: boolean } & QuestionProps) {
  const card = item.kind === "question" || item.kind === "done";
  return (
    <li className={cn("group flex gap-3", opensRun ? "mt-5" : "mt-1.5")}>
      <div className="w-7 shrink-0">{opensRun && <Avatar name={item.who} />}</div>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        {opensRun && (
          <span className="flex items-baseline gap-2">
            <span className="font-mono text-[12px] font-semibold text-ink-primary">{item.who}</span>
            <Mono className="text-[11px] text-ink-faint">{clockTime(item.at)}</Mono>
          </span>
        )}
        {card ? (
          <div
            role={answering ? "button" : undefined}
            tabIndex={answering ? 0 : undefined}
            onClick={answering ? onOpenQuestion : undefined}
            onKeyDown={answering ? (event) => (event.key === "Enter" || event.key === " ") && onOpenQuestion() : undefined}
            className={cn(
              "flex flex-col gap-2.5 rounded-lg border px-4 py-3",
              item.kind === "question" ? "border-warning/50" : "border-success/50",
              answering && "cursor-pointer transition-colors duration-(--dur-fast) hover:border-warning hover:bg-surface-1",
            )}
          >
            <span
              className={cn(
                "inline-flex items-center gap-1.5 text-[11px] font-semibold tracking-wide uppercase [&>svg]:size-3.5",
                item.kind === "question" ? "text-warning" : "text-success",
              )}
            >
              {item.kind === "question" ? <MessageCircleQuestionMark /> : <CircleCheck />}
              {item.kind === "question" ? (answering ? "Asks you" : "Asked you") : "Done"}
            </span>
            <Long text={item.text} taskId={taskId} />
            {answering && (
              <span className="flex items-center gap-3 border-t border-warning/30 pt-2.5 text-[12px] font-medium text-warning">
                Answer in Questions{hasPage && <span className="font-normal text-ink-muted">· it made a page for this</span>}
                <ChevronRight className="ml-auto size-4" aria-hidden />
              </span>
            )}
          </div>
        ) : (
          <Long text={item.text} taskId={taskId} />
        )}
      </div>
    </li>
  );
}

// The work between two messages: one quiet line, which opens into what
// happened, each step with its own text.
function Work({ items, taskId }: { items: ChatItem[]; taskId: string }) {
  const [open, setOpen] = useState(false);
  const summary = items
    .slice(-3)
    .map((item) => (item.kind === "handoff" ? `handed to ${item.to}` : item.kind === "report" ? `${item.who} reported` : "a note"))
    .join(" · ");
  const failed = items.some((item) => item.tone === "danger");
  return (
    <li className="mt-4 ml-10">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="-ml-2 inline-flex max-w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-[12px] text-ink-muted hover:bg-surface-1 hover:text-ink-primary"
      >
        <ChevronRight className={cn("size-3.5 shrink-0 transition-transform duration-(--dur-fast)", open && "rotate-90")} />
        <span className="shrink-0 font-medium">
          {items.length} {items.length === 1 ? "update" : "updates"}
        </span>
        <span className="truncate text-ink-faint">{summary}</span>
        {failed && <TriangleAlert className="size-3.5 shrink-0 text-danger" aria-label="something went wrong" />}
      </button>
      {open && (
        <ol className="relative mt-2 flex flex-col gap-3 pl-5 before:absolute before:top-1 before:bottom-1 before:left-[6px] before:w-px before:bg-divider">
          {items.map((item) => (
            <WorkStep key={item.key} item={item} taskId={taskId} />
          ))}
        </ol>
      )}
    </li>
  );
}

function WorkStep({ item, taskId }: { item: ChatItem; taskId: string }) {
  const icon = item.kind === "handoff" ? <ArrowRight /> : item.kind === "report" ? <FileText /> : <TriangleAlert />;
  return (
    <li className="relative">
      <span
        className={cn(
          "absolute top-0.5 -left-5 flex size-3.5 items-center justify-center bg-surface-0 text-ink-faint [&>svg]:size-3",
          item.tone === "danger" && "text-danger",
        )}
      >
        {icon}
      </span>
      <span className="flex items-baseline gap-2 text-[12px]">
        <span className="font-mono font-semibold text-ink-secondary">
          {item.kind === "handoff" ? `→ ${item.to}` : item.kind === "report" ? item.who : "Agentry"}
        </span>
        <Mono className="text-[11px] text-ink-faint">{clockTime(item.at)}</Mono>
      </span>
      <Long text={item.text} taskId={taskId} short className="mt-1 text-ink-secondary [&_*]:text-[12px]" />
    </li>
  );
}

// Long text, cut to a height until you ask for all of it. A link to a file
// of the task, as agents link what they made, opens it in the task's
// Workspace, in a new tab, as it reads: a document rendered, a diagram drawn.
function Long({ text, taskId, short = false, className }: { text: string; taskId: string; short?: boolean; className?: string }) {
  const [all, setAll] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const [cut, setCut] = useState(false);
  useLayoutEffect(() => {
    if (box.current) setCut(box.current.scrollHeight > box.current.clientHeight + 4);
  }, [text]);
  return (
    <div className={className}>
      <div ref={box} className={cn(!all && (short ? "max-h-24" : "max-h-80"), "overflow-hidden")}>
        <Markdown text={text} link={(href) => linkInto(taskId, href)} />
      </div>
      {(cut || all) && (
        <button
          type="button"
          onClick={() => setAll((a) => !a)}
          className="mt-1 cursor-pointer text-[11px] font-medium text-ink-muted hover:text-ink-primary"
        >
          {all ? "Show less" : "Show more"}
        </button>
      )}
    </div>
  );
}

// The coordinator at work: three dots under its avatar.
function Typing({ name }: { name: string }) {
  return (
    <li className="mt-5 flex items-center gap-3" aria-label={`${name} is thinking`}>
      <Avatar name={name} />
      <span className="flex h-8 items-center gap-1 rounded-lg bg-surface-2 px-3">
        {[0, 150, 300].map((delay) => (
          <span key={delay} className="size-1.5 animate-bounce rounded-full bg-ink-muted" style={{ animationDelay: `${delay}ms` }} />
        ))}
      </span>
    </li>
  );
}
