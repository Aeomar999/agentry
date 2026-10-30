import { useEffect, useState } from "react";
import { Save } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { AgentName } from "@/components/molecules/AgentName";
import { CodeViewer } from "@/components/molecules/CodeViewer";
import { Panel } from "@/components/molecules/Panel";
import { api } from "@/api/client";
import { useResource } from "@/live/use-resource";
import { timeAgo } from "@/lib/time";
import { useToast } from "@/shell/Toasts";

// What an agent knows: notes it keeps as it works on tasks, part of every
// brief. You can help it here: add what it should know, fix what's wrong.
// Every change, the agent's or yours, is kept, newest first.

interface Change {
  by: string;
  at: string;
  knowledge: string;
}

export function KnowledgeEditor({ agent, knowledge }: { agent: string; knowledge: string }) {
  const notify = useToast();
  const history = useResource<Change[]>(`/api/agents/${encodeURIComponent(agent)}/knowledge`).data ?? [];
  const [draft, setDraft] = useState(knowledge);
  const [busy, setBusy] = useState(false);
  const [editedFrom, setEditedFrom] = useState(knowledge); // what the draft started from
  useEffect(() => {
    // The agent learned something while you weren't editing: show it.
    if (draft === editedFrom) setDraft(knowledge);
    setEditedFrom(knowledge);
  }, [knowledge]); // only a new knowledge from the server; the draft is read, not watched

  const changed = draft.trim() !== knowledge.trim();
  async function save() {
    setBusy(true);
    try {
      await api("PUT", `/api/agents/${encodeURIComponent(agent)}/knowledge`, { knowledge: draft });
      notify("success", `Saved what ${agent} knows`, "It reads it from its next turn.");
    } catch (failure) {
      notify("danger", "That wasn't saved", (failure as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel
      flush
      title="Knowledge"
      aside={
        <Button size="sm" variant={changed ? "primary" : "secondary"} icon={<Save />} disabled={!changed || busy} onClick={save}>
          Save
        </Button>
      }
    >
      <p className="border-b border-divider px-4 py-2.5 text-[12px] text-ink-muted">
        What {agent} has learned on earlier tasks, part of every brief. It keeps it up to date as it works; add what it should know.
      </p>
      <CodeViewer text={draft} language="markdown" minHeight={160} maxHeight={800} onChange={setDraft} />
      {history.length > 0 && (
        <ol className="border-t border-divider">
          {history.slice(0, 8).map((change, i) => (
            <li key={i} className="flex items-center gap-3 border-b border-divider px-4 py-1.5 text-[12px] last:border-b-0">
              {change.by === "you" ? (
                <span className="font-mono text-ink-secondary">you</span>
              ) : (
                <AgentName name={change.by} className="text-ink-secondary" />
              )}
              <span className="text-ink-faint">{timeAgo(change.at)}</span>
              <span className="min-w-0 flex-1 truncate text-ink-muted">{change.knowledge.split("\n")[0] || "(cleared)"}</span>
            </li>
          ))}
        </ol>
      )}
    </Panel>
  );
}
