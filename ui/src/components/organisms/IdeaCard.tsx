import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Check, X } from "lucide-react";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { AgentName } from "@/components/molecules/AgentName";
import { Markdown } from "@/components/molecules/Markdown";
import { Panel } from "@/components/molecules/Panel";
import { cn } from "@/lib/cn";
import type { Idea } from "@/api/types";

export const SIZES: Record<Idea["size"], string> = { S: "small", M: "medium", L: "large" };

// IdeaCard is one open idea, the way its session wrote it up: the problem,
// the feature, why now, its risks, and which viewpoints backed or doubted
// it, and why Agentry didn't accept it for you. Accept makes it a task in
// your Backlog; Dismiss puts it away.
export function IdeaCard({ idea, onAccept, onDismiss }: { idea: Idea; onAccept: () => void; onDismiss: () => void }) {
  return (
    <Panel
      title={<span className="truncate">{idea.title}</span>}
      aside={
        <>
          {idea.kind === "epic" && <Badge variant="info">epic</Badge>}
          <Badge>{SIZES[idea.size] ?? idea.size}</Badge>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Part label="Problem" text={idea.problem} />
        <Part label="Proposal" text={idea.proposal} />
        <Part label="Why now" text={idea.whyNow} />
        <Part label="Risks" text={idea.risks} />
        <div className="flex flex-wrap gap-x-8 gap-y-3">
          <Voices label="Backed by" agents={idea.backedBy} />
          <Voices label="Doubted by" agents={idea.doubtedBy} />
        </div>
        {idea.notAutoAccepted && <p className="text-[12px] text-ink-muted">Not auto-accepted: {idea.notAutoAccepted}</p>}
        <div className="flex flex-wrap items-center gap-2 border-t border-divider pt-3">
          <Button size="sm" icon={<Check />} onClick={onAccept}>
            Accept
          </Button>
          <Button size="sm" variant="ghost" icon={<X />} onClick={onDismiss}>
            Dismiss
          </Button>
          <span className="ml-auto flex flex-wrap gap-x-3 text-[12px] text-ink-muted">
            {idea.epic && (
              <span>
                follows up <TaskLink id={idea.epic} />
              </span>
            )}
            <span>
              from <TaskLink id={idea.task} />
            </span>
          </span>
        </div>
      </div>
    </Panel>
  );
}

// TaskLink goes to a task, showing its id in mono or, given children such as
// its title, those in the normal font.
export function TaskLink({ id, children }: { id: string; children?: ReactNode }) {
  return (
    <Link
      to={`/tasks/${encodeURIComponent(id)}`}
      className={cn("text-ink-secondary underline-offset-2 hover:text-ink-primary hover:underline", children === undefined && "font-mono")}
    >
      {children ?? id}
    </Link>
  );
}

function Part({ label, text }: { label: string; text: string }) {
  if (!text.trim()) return null;
  return (
    <div className="flex flex-col gap-1">
      <h3 className="text-[10.5px] font-semibold tracking-wider text-ink-faint uppercase">{label}</h3>
      <Markdown text={text} className="text-ink-secondary" />
    </div>
  );
}

function Voices({ label, agents }: { label: string; agents: string[] }) {
  return (
    <div className="flex flex-col gap-1">
      <h3 className="text-[10.5px] font-semibold tracking-wider text-ink-faint uppercase">{label}</h3>
      {agents.length ? (
        <span className="flex flex-wrap gap-x-3 gap-y-1 text-[12px] text-ink-secondary">
          {agents.map((name) => (
            <AgentName key={name} name={name} />
          ))}
        </span>
      ) : (
        <span className="text-[12px] text-ink-faint">no one</span>
      )}
    </div>
  );
}
