import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ListPlus, ListTree, Plus } from "lucide-react";
import type { TaskDetail, TaskSummary } from "@/api/types";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import { StoryProgress } from "@/components/molecules/StoryProgress";
import { tokens } from "@/lib/tokens";
import { waitingLine } from "@/lib/epics";
import { shownEpic } from "@/lib/shown-id";
import { fileUnder } from "@/lib/epic-api";
import { useNewTask } from "@/lib/new-task";
import { rankAt } from "@/lib/rank";
import { api } from "@/api/client";
import { useResource } from "@/live/use-resource";
import { Button } from "@/components/atoms/Button";
import { Checkbox } from "@/components/atoms/Checkbox";
import { Dialog } from "@/components/molecules/Dialog";
import { SearchField } from "@/components/molecules/SearchField";
import { useToast } from "@/shell/Toasts";

// An epic's stories, in your priority order (drag a row to change it), the
// finished ones last: who runs each, what it builds on, and how it stands. Tasks there already can be
// added as stories, and a story taken out; New story makes one in the epic.

const COLUMNS: Column<TaskSummary>[] = [
  { header: "Story", className: "w-24", render: (story) => <Mono className="text-ink-muted">{story.id}</Mono> },
  {
    header: "Title",
    className: "w-full max-w-0",
    render: (story) => (
      <div className="min-w-0 py-0.5">
        <p className="truncate font-medium text-ink-primary">{story.title}</p>
        <p className="truncate text-[12px] text-ink-muted">
          {story.status === "needs you" ? (
            <span className="text-warning">{story.needsYou}</span>
          ) : (
            waitingLine(story) || (story.currentAgent ? `${story.currentAgent} is working` : story.status === "done" ? "Done" : "")
          )}
        </p>
      </div>
    ),
  },
  {
    header: "Builds on",
    className: "w-32 max-lg:hidden",
    render: (story) => <Mono className="text-ink-muted">{story.after.join(", ") || "nothing"}</Mono>,
  },
  { header: "Status", className: "w-28", render: (story) => <TaskStatusBadge status={story.status} /> },
  {
    header: "Agent",
    className: "w-32 max-lg:hidden",
    render: (story) => (story.currentAgent ? <AgentName name={story.currentAgent} /> : <Mono className="text-ink-faint">none</Mono>),
  },
  { header: "Tokens", className: "w-24 text-right max-lg:hidden", render: (story) => <Mono>{tokens(story.tokensUsed)}</Mono> },
];

export function StoryTable({ epic, onChanged }: { epic: TaskDetail; onChanged: () => void }) {
  const navigate = useNavigate();
  const notify = useToast();
  const [adding, setAdding] = useState(false);
  const newTask = useNewTask();
  const listed = [...(epic.storyList ?? [])].sort((a, b) => a.rank - b.rank);
  const stories = [...listed.filter((story) => story.status !== "done"), ...listed.filter((story) => story.status === "done")];

  async function reorder(story: TaskSummary, at: number) {
    const rank = rankAt(stories, story, at);
    if (rank === null) return;
    try {
      await api("PUT", `/api/tasks/${encodeURIComponent(story.id)}/rank`, { rank });
      onChanged();
    } catch (failure) {
      notify("danger", "The order wasn't saved", (failure as Error).message);
    }
  }
  const used = stories.reduce((sum, story) => sum + story.tokensUsed, 0);
  const open = epic.status !== "done";

  async function takeOut(story: TaskSummary) {
    try {
      await fileUnder(story.id, "");
      notify("success", `${story.id} is out of ${shownEpic(epic.id)}`);
      onChanged();
    } catch (failure) {
      notify("danger", "It is still in the epic", (failure as Error).message);
    }
  }

  const columns: Column<TaskSummary>[] = open
    ? [
        ...COLUMNS,
        {
          header: "",
          className: "w-20 text-right",
          render: (story) => (
            <button
              type="button"
              onClick={(event) => (event.stopPropagation(), takeOut(story))}
              className="cursor-pointer text-[12px] text-ink-faint hover:text-danger"
            >
              Take out
            </button>
          ),
        },
      ]
    : COLUMNS;

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-4 border-b border-divider px-6 py-3 text-[12px] text-ink-muted">
        <StoryProgress done={epic.storiesDone} total={epic.stories} />
        {stories.length > 0 && (
          <span>
            {tokens(used)} tokens across its stories, {tokens(epic.tokensUsed)} for the epic itself
          </span>
        )}
        {open && (
          <span className="ml-auto flex gap-2">
            <Button size="sm" icon={<ListPlus />} onClick={() => setAdding(true)}>
              Add tasks
            </Button>
            <Button size="sm" icon={<Plus />} onClick={() => newTask("story", epic.id)}>
              New story
            </Button>
          </span>
        )}
      </div>
      <div className="min-h-0 flex-1 scrollbar-thin overflow-y-auto">
        {stories.length ? (
          <ResourceTable
            columns={columns}
            rows={stories}
            rowKey={(story) => story.id}
            onOpen={(story) => navigate(`/tasks/${story.id}`)}
            onReorder={open ? reorder : undefined}
          />
        ) : (
          <EmptyState icon={<ListTree />} title="No stories yet">
            The coordinator splits the epic into stories, or add tasks you have already. Each story is a task of its own, with its own
            branch and checks.
          </EmptyState>
        )}
      </div>
      {adding && (
        <AddStories
          epic={epic}
          onClose={() => setAdding(false)}
          onAdded={() => {
            setAdding(false);
            onChanged();
          }}
        />
      )}
    </div>
  );
}

// AddStories picks tasks there already to file under the epic. A task in
// another epic moves; epics themselves can't be stories.
function AddStories({ epic, onClose, onAdded }: { epic: TaskDetail; onClose: () => void; onAdded: () => void }) {
  const notify = useToast();
  const [picked, setPicked] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const words = search.toLowerCase().split(/\s+/).filter(Boolean);
  const candidates = (useResource<TaskSummary[]>("/api/tasks").data ?? []).filter(
    (task) =>
      task.kind !== "epic" &&
      task.epic !== epic.id &&
      words.every((word) => `${task.id} ${task.title} ${task.labels.join(" ")}`.toLowerCase().includes(word)),
  );

  async function add() {
    setBusy(true);
    const failed: string[] = [];
    for (const id of picked) {
      try {
        await fileUnder(id, epic.id);
      } catch (failure) {
        failed.push(`${id}: ${(failure as Error).message}`);
      }
    }
    if (failed.length) notify("danger", "Some weren't added", failed.join("\n"));
    else notify("success", `Added ${picked.length} to ${shownEpic(epic.id)}`, "The epic's coordinator reads it at its next turn.");
    onAdded();
  }

  return (
    <Dialog title={`Add tasks to ${epic.title}`} onClose={onClose} wide>
      <div className="flex flex-col gap-3">
        <SearchField value={search} onChange={setSearch} placeholder="Find tasks" />
        <div className="flex max-h-96 scrollbar-thin flex-col gap-2 overflow-y-auto">
          {candidates.map((task) => (
            <Checkbox
              key={task.id}
              checked={picked.includes(task.id)}
              onChange={(on) => setPicked(on ? [...picked, task.id] : picked.filter((id) => id !== task.id))}
              label={`${task.id} · ${task.title}`}
              hint={[task.status, task.epic && `moves from ${shownEpic(task.epic)}`].filter(Boolean).join(" · ")}
            />
          ))}
          {candidates.length === 0 && <p className="py-3 text-[12px] text-ink-faint">No other tasks to add.</p>}
        </div>
        <div className="flex gap-2">
          <Button variant="primary" icon={<ListPlus />} disabled={busy || picked.length === 0} onClick={add}>
            Add {picked.length || ""}
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
