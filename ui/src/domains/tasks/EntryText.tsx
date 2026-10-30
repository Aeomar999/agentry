import { CodeBox } from "@/components/molecules/CodeBox";
import { Fold } from "@/components/molecules/Fold";
import { Markdown, MarkdownProps } from "@/components/molecules/Markdown";

// The text of an entry in a task's Activity or Handoffs (a brief, a report,
// a question, a note), so both tabs draw text one way: compact Markdown,
// links into the task (useTaskLinks), fenced code in a CodeBox, and folded
// past a few lines. Callers fold briefs, reports, questions and the rest at
// 10 lines, Handoffs briefs at 8 and notes at 4.
//
// Like CodeBox, it must not import lib/monaco.

export type TaskLinks = Pick<MarkdownProps, "link" | "refs">;

const LINE = 13 * 1.6; // px, compact Markdown's line height

export function EntryText({ text, links, lines, kind }: { text: string; links: TaskLinks; lines: number; kind: string }) {
  return (
    <Fold lines={lines} lineHeight={LINE} kind={kind}>
      <Markdown text={text} marks {...links} code={(code, language) => <CodeBox code={code} language={language} />} />
    </Fold>
  );
}
