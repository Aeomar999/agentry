import { useState } from "react";
import { Check, Play, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { Field } from "@/components/molecules/Field";
import { LimitSelect } from "@/components/molecules/LimitSelect";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { api } from "@/api/client";
import type { ServerConfig } from "@/api/types";
import { LimitChoice, SCALES, choiceOf, limitOf, sent } from "@/lib/limits";
import { useResource } from "@/live/use-resource";
import { COLUMN_TITLES } from "@/lib/columns";
import { TaskLink } from "./IdeaCard";
import type { Idea, IdeaAcceptance } from "./types";
import { useToast } from "@/components/organisms/Toasts";

const path = (idea: Idea, action: "accept" | "dismiss") => `/api/ideas/${encodeURIComponent(idea.id)}/${action}`;

// AcceptIdeaDialog makes the idea a task or an epic, the kind it suggests
// unless you pick the other, with your note and token budget, in your
// Backlog or started at once. It stays open to link to what was made, and
// says why when the idea couldn't be marked.
export function AcceptIdeaDialog({ idea, onClose }: { idea: Idea; onClose: () => void }) {
  const notify = useToast();
  const config = useResource<ServerConfig>("/api/config").data;
  const [kind, setKind] = useState<"task" | "epic">(idea.kind === "epic" ? "epic" : "task");
  const [note, setNote] = useState("");
  const [budget, setBudget] = useState<LimitChoice | null>(null); // null: the server's default
  const [busy, setBusy] = useState<"" | "accept" | "start">("");
  const [refused, setRefused] = useState("");
  const [made, setMade] = useState<IdeaAcceptance | null>(null);

  async function accept(start: boolean) {
    const limit = budget ? limitOf(SCALES.budget, budget) : undefined;
    if (Number.isNaN(limit)) {
      setRefused("Give the budget as a number of tokens, or pick a preset.");
      return;
    }
    setBusy(start ? "start" : "accept");
    setRefused("");
    try {
      const asked = { kind, note, ...(limit !== undefined && { budget: sent(limit) }), ...(start && { start }) };
      const got = await api<IdeaAcceptance>("POST", path(idea, "accept"), asked);
      setMade(got);
      if (got.accepted) notify("success", `Made ${got.task} from the idea`, placeOf(got.column));
    } catch (failure) {
      setRefused((failure as Error).message);
    }
    setBusy("");
  }

  return (
    <Dialog title={`Accept “${idea.title}”`} onClose={onClose}>
      {made ? (
        <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
          <p>
            Made <TaskLink id={made.task} /> from it. {placeOf(made.column)}
          </p>
          {!made.accepted && <Banner title="The idea wasn't marked accepted">{made.refused}</Banner>}
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <Field
            label="Make it"
            labelId="idea-kind-label"
            hint={`The session suggests ${idea.kind === "epic" ? "an epic: too big for one change, split into stories" : "one task: one change, on one branch"}.`}
          >
            <SegmentedControl
              active={kind}
              onChange={(id) => setKind(id as typeof kind)}
              labelledBy="idea-kind-label"
              segments={[
                { id: "task", label: "A task" },
                { id: "epic", label: "An epic" },
              ]}
            />
          </Field>
          <Field label="Note" htmlFor="idea-note" hint="Optional. Kept with the idea, in its history.">
            <TextArea id="idea-note" autoFocus rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
          {config && (
            <Field
              label="Token budget"
              htmlFor="idea-budget"
              hint="Accept and start spends tokens straight away. At this many it stops and asks you to raise it or wrap up."
            >
              <LimitSelect
                id="idea-budget"
                scale={SCALES.budget}
                choice={budget ?? choiceOf(SCALES.budget, config.budget)}
                onChange={setBudget}
                defaultLimit={config.budget}
              />
            </Field>
          )}
          {refused && <Banner title="It wasn't accepted">{refused}</Banner>}
          <div className="flex gap-2">
            <Button variant="primary" icon={<Check />} disabled={busy !== ""} onClick={() => accept(false)}>
              {busy === "accept" ? "Accepting…" : kind === "epic" ? "Accept as an epic" : "Accept as a task"}
            </Button>
            <Button icon={<Play />} disabled={busy !== ""} onClick={() => accept(true)}>
              {busy === "start" ? "Starting…" : "Accept and start"}
            </Button>
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </Dialog>
  );
}

// placeOf says where the idea's task is, from the column the server
// answers, not the button pressed: a retry after a failed accept finds the
// task where it was made. Empty when the column can't be read.
function placeOf(column: IdeaAcceptance["column"]): string {
  switch (column) {
    case "":
      return "";
    case "backlog":
      return "In your Backlog until you start it.";
    case "in-progress":
    case "review":
    case "shipping":
      return `Started, now in ${COLUMN_TITLES[column]}.`;
    case "done":
      return `${COLUMN_TITLES[column]}.`;
  }
  return `In ${COLUMN_TITLES[column]}.`;
}

// DismissIdeaDialog puts the idea away, into the history, with your reason.
export function DismissIdeaDialog({ idea, onClose }: { idea: Idea; onClose: () => void }) {
  const notify = useToast();
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState("");

  async function dismiss() {
    setBusy(true);
    setRefused("");
    try {
      await api("POST", path(idea, "dismiss"), { note });
      notify("success", `Dismissed “${idea.title}”`, "It is in the history.");
      onClose();
    } catch (failure) {
      setRefused((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <Dialog title={`Dismiss “${idea.title}”`} onClose={onClose}>
      <div className="flex flex-col gap-4">
        <Field label="Reason" htmlFor="idea-reason" hint="Optional. Kept with the idea, in its history.">
          <TextArea id="idea-reason" autoFocus rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        {refused && <Banner title="It wasn't dismissed">{refused}</Banner>}
        <div className="flex gap-2">
          <Button variant="danger" icon={<X />} disabled={busy} onClick={dismiss}>
            Dismiss
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Keep
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
