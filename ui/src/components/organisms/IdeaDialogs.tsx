import { useState } from "react";
import { Check, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { Field } from "@/components/molecules/Field";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { api } from "@/api/client";
import type { Idea, IdeaAcceptance } from "@/api/types";
import { TaskLink } from "./IdeaCard";
import { useToast } from "./Toasts";

const path = (idea: Idea, action: "accept" | "dismiss") => `/api/ideas/${encodeURIComponent(idea.id)}/${action}`;

// AcceptIdeaDialog makes the idea a task or an epic in your Backlog, the
// kind it suggests unless you pick the other, with your note. It stays open
// to link to what was made, and says why when the idea couldn't be marked.
export function AcceptIdeaDialog({ idea, onClose }: { idea: Idea; onClose: () => void }) {
  const notify = useToast();
  const [kind, setKind] = useState<"task" | "epic">(idea.kind === "epic" ? "epic" : "task");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState("");
  const [made, setMade] = useState<IdeaAcceptance | null>(null);

  async function accept() {
    setBusy(true);
    setRefused("");
    try {
      const got = await api<IdeaAcceptance>("POST", path(idea, "accept"), { kind, note });
      setMade(got);
      if (got.accepted) notify("success", `Made ${got.task} from the idea`, "In your Backlog until you start it.");
    } catch (failure) {
      setRefused((failure as Error).message);
    }
    setBusy(false);
  }

  return (
    <Dialog title={`Accept “${idea.title}”`} onClose={onClose}>
      {made ? (
        <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
          <p>
            Made <TaskLink id={made.task} /> from it, in your Backlog until you start it.
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
            htmlFor="idea-kind"
            hint={`The session suggests ${idea.kind === "epic" ? "an epic: too big for one change, split into stories" : "one task: one change, on one branch"}.`}
          >
            <div id="idea-kind">
              <SegmentedControl
                active={kind}
                onChange={(id) => setKind(id as typeof kind)}
                segments={[
                  { id: "task", label: "A task" },
                  { id: "epic", label: "An epic" },
                ]}
              />
            </div>
          </Field>
          <Field label="Note" htmlFor="idea-note" hint="Optional. Kept with the idea, in its history.">
            <TextArea id="idea-note" rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
          {refused && <Banner title="It wasn't accepted">{refused}</Banner>}
          <div className="flex gap-2">
            <Button variant="primary" icon={<Check />} disabled={busy} onClick={accept}>
              {busy ? "Accepting…" : kind === "epic" ? "Accept as an epic" : "Accept as a task"}
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
