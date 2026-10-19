import { useState } from "react";
import { Lightbulb, Users } from "lucide-react";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { Checkbox } from "@/components/atoms/Checkbox";
import { Radio } from "@/components/atoms/Radio";
import { Banner } from "@/components/molecules/Banner";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { EmptyState } from "@/components/molecules/EmptyState";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { changeSettings } from "@/api/client";
import type { AutoAcceptRule, Settings, TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { VISIONARIES } from "@/lib/teams";
import { IdeaCard, TaskLink } from "./IdeaCard";
import { AcceptIdeaDialog, DismissIdeaDialog } from "./IdeaDialogs";
import type { Idea } from "./types";
import { useAskVisionaries } from "./visionaries";

// Ideas are the features Visionaries sessions suggest. The open ones wait
// here, grouped by the session that suggested them, newest first (the
// server's order), for you to accept, which makes a task or an epic in your
// Backlog, or dismiss. What you decided stays below, in the history, with
// your note. Ask the visionaries starts a new session on a theme or a
// question, adding the team first if you don't have it. Settings here say
// whether closing an epic starts a session about its follow-ups, and whether
// Agentry accepts, and starts, the ideas the Visionaries agreed on, by your
// rule; those show in the history as Agentry's, and an open idea says why
// Agentry didn't accept it.

export function IdeasPage() {
  useBreadcrumb({ label: "Ideas" });
  const { data, error } = useResource<Idea[]>("/api/ideas");
  const ideas = data ?? [];
  const titles = new Map((useResource<TaskSummary[]>("/api/tasks").data ?? []).map((task) => [task.id, task.title]));
  const visionaries = useAskVisionaries();
  const [acting, setActing] = useState<{ idea: Idea; action: "accept" | "dismiss" } | null>(null);

  const open = ideas.filter((idea) => idea.state === "open");
  const decided = ideas.filter((idea) => idea.state !== "open");
  const ask = visionaries.has ? "Ask the visionaries" : "Add the Visionaries team";

  return (
    <>
      <PageHeader
        title="Ideas"
        meta="Features the Visionaries suggest, from sessions on a theme or a question. Accept one to make it a task in your Backlog."
        actions={[
          {
            label: visionaries.adding ? "Adding the team…" : ask,
            icon: visionaries.has ? <Lightbulb /> : <Users />,
            onClick: visionaries.ask,
            primary: true,
            disabled: !visionaries.ready || visionaries.adding,
          },
        ]}
      />
      <div className="flex flex-col gap-8 p-6">
        {error && <Banner title="Couldn't load the ideas">{error}</Banner>}
        {visionaries.missingProfiles.length > 0 && (
          <MissingProfilesOffer
            team={VISIONARIES}
            profiles={visionaries.missingProfiles}
            adding={visionaries.adding}
            onAdd={visionaries.askAddingProfiles}
          />
        )}
        {visionaries.error && <Banner title="The Visionaries team wasn't added">{visionaries.error}</Banner>}
        <IdeaSettings />
        {data && ideas.length === 0 && (
          <EmptyState icon={<Lightbulb />} title="No ideas yet">
            <div className="flex flex-col items-center gap-3">
              <p>
                Ask the visionaries about a theme or a question. They read what exists, argue it out from several viewpoints, and the ideas
                that survive come here.
              </p>
              <Button
                icon={visionaries.has ? <Lightbulb /> : <Users />}
                disabled={!visionaries.ready || visionaries.adding}
                onClick={visionaries.ask}
              >
                {ask}
              </Button>
            </div>
          </EmptyState>
        )}
        {data && ideas.length > 0 && open.length === 0 && (
          <p className="text-[13px] text-ink-muted">No open ideas: you have decided on every one. Ask the visionaries for more.</p>
        )}
        {sessionsOf(open).map(({ task, ideas }) => (
          <section key={task} className="flex flex-col gap-3">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <h2 className="font-display text-[17px] font-bold tracking-tight text-ink-primary [font-stretch:85%]">
                {titles.get(task) ?? "A Visionaries session"}
              </h2>
              <span className="text-[12px] text-ink-muted">
                <TaskLink id={task} /> · {ideas.length} {ideas.length === 1 ? "idea" : "ideas"}
                {ideas[0].about && (
                  <>
                    {" "}
                    · after you closed <TaskLink id={ideas[0].about}>{ideas[0].aboutTitle || undefined}</TaskLink>
                  </>
                )}
              </span>
            </div>
            <div className="grid items-start gap-3 xl:grid-cols-2">
              {ideas.map((idea) => (
                <IdeaCard
                  key={idea.id}
                  idea={idea}
                  onAccept={() => setActing({ idea, action: "accept" })}
                  onDismiss={() => setActing({ idea, action: "dismiss" })}
                />
              ))}
            </div>
          </section>
        ))}
        {decided.length > 0 && (
          <Panel title="History" aside={`${decided.length} decided`} flush>
            <ul className="divide-y divide-divider">
              {decided.map((idea) => (
                <HistoryRow key={idea.id} idea={idea} />
              ))}
            </ul>
          </Panel>
        )}
      </div>
      {acting?.action === "accept" && <AcceptIdeaDialog idea={acting.idea} onClose={() => setActing(null)} />}
      {acting?.action === "dismiss" && <DismissIdeaDialog idea={acting.idea} onClose={() => setActing(null)} />}
    </>
  );
}

// The settings' values before you change any, and while they can't be read.
const DEFAULTS: Settings = {
  visionariesOnEpicClose: true,
  autoAcceptIdeas: false,
  autoStartIdeas: false,
  autoAcceptRule: "everyoneBacked",
};

// The rules for which ideas Agentry accepts for you, the strict one first.
const RULES: { rule: AutoAcceptRule; label: string; hint: string }[] = [
  { rule: "everyoneBacked", label: "Everyone backed it", hint: "Every viewpoint backed it, no one doubted it." },
  { rule: "nobodyDoubted", label: "Nobody doubted it", hint: "No one doubted it and at least two viewpoints backed it." },
];

// IdeaSettings are the settings about ideas: whether closing an epic starts
// a Visionaries session about its follow-ups, and whether Agentry accepts,
// and starts, the ideas the Visionaries agreed on, and by which rule. Each
// change names only its setting. They wait for the settings before you can
// change them, show what you chose while the server saves it and what it
// saved until the settings load again (they only reload when the event log
// moves), and the defaults when the settings can't be read. Starting them
// counts only with accepting them, so it reads as off and can't be changed
// while that is off; the rule can't be changed then either.
function IdeaSettings() {
  const { data, error } = useResource<Settings>("/api/settings");
  const [saving, setSaving] = useState<Partial<Settings> | null>(null);
  const [saved, setSaved] = useState<Settings | null>(null);
  const [failed, setFailed] = useState("");
  const [seen, setSeen] = useState(data);
  if (seen !== data) {
    setSeen(data);
    setSaved(null);
  }

  const value = <K extends keyof Settings>(name: K): Settings[K] => saving?.[name] ?? saved?.[name] ?? data?.[name] ?? DEFAULTS[name];
  const busy = saving !== null || (!data && !error);

  async function change(setting: Partial<Settings>) {
    setSaving(setting);
    setFailed("");
    try {
      setSaved(await changeSettings(setting));
    } catch (failure) {
      setSaved(null);
      setFailed((failure as Error).message);
    } finally {
      setSaving(null);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-start gap-x-3">
        <div className="flex flex-col gap-3">
          <Checkbox
            checked={value("visionariesOnEpicClose")}
            onChange={(on) => change({ visionariesOnEpicClose: on })}
            disabled={busy}
            label="Ask the visionaries for follow-ups when you close an epic"
            hint="A small session about what the epic delivered and what could come next, with the server's default budget and turn limit. Its ideas show here, linked to the epic."
          />
          <Checkbox
            checked={value("autoAcceptIdeas")}
            onChange={(on) => change({ autoAcceptIdeas: on })}
            disabled={busy}
            label="Auto-accept ideas the Visionaries agree on"
            hint="Agentry accepts for you an idea that qualifies, into your Backlog, unless it repeats one you have already. The others wait here, saying why."
          />
          <fieldset className="flex flex-col gap-2 pl-6.5">
            <legend className="mb-2 text-[12px] font-medium text-ink-secondary">Which ideas qualify</legend>
            {RULES.map(({ rule, label, hint }) => (
              <Radio
                key={rule}
                name="auto-accept-rule"
                checked={value("autoAcceptRule") === rule}
                onChange={() => change({ autoAcceptRule: rule })}
                disabled={busy || !value("autoAcceptIdeas")}
                label={label}
                hint={hint}
              />
            ))}
          </fieldset>
          <div className="pl-6.5">
            <Checkbox
              checked={value("autoAcceptIdeas") && value("autoStartIdeas")}
              onChange={(on) => change({ autoStartIdeas: on })}
              disabled={busy || !value("autoAcceptIdeas")}
              label="and start them"
              hint="Moves each task it accepted to In progress, as Start does, instead of leaving it in your Backlog."
            />
          </div>
        </div>
        {saving !== null && <span className="text-[12px] text-ink-muted">Saving…</span>}
      </div>
      {error && <Banner title="Couldn't read the settings">{error}</Banner>}
      {failed && <Banner title="The setting wasn't saved">{failed}</Banner>}
    </div>
  );
}

// An idea decided on, by you or, when the Visionaries agreed, by Agentry:
// what became of it, and what was said.
function HistoryRow({ idea }: { idea: Idea }) {
  return (
    <li className="flex flex-col gap-1 px-4 py-3">
      <span className="flex flex-wrap items-center gap-2">
        <Badge variant={idea.state === "accepted" ? "success" : "default"}>{idea.state}</Badge>
        {idea.autoAccepted && (
          <span title={idea.note || undefined}>
            <Badge variant="info">Auto-accepted</Badge>
          </span>
        )}
        <span className="text-[13px] font-medium text-ink-primary">{idea.title}</span>
        <span className="ml-auto flex gap-3 text-[12px] text-ink-muted">
          {idea.became && (
            <span>
              became <TaskLink id={idea.became} />
            </span>
          )}
          <span>
            from <TaskLink id={idea.task} />
          </span>
        </span>
      </span>
      {idea.note && <p className="text-[12.5px] whitespace-pre-wrap text-ink-secondary">{idea.note}</p>}
    </li>
  );
}

// sessionsOf groups ideas by the session they came from, keeping their order.
function sessionsOf(ideas: Idea[]): { task: string; ideas: Idea[] }[] {
  const sessions: { task: string; ideas: Idea[] }[] = [];
  for (const idea of ideas) {
    let session = sessions.find((s) => s.task === idea.task);
    if (!session) sessions.push((session = { task: idea.task, ideas: [] }));
    session.ideas.push(idea);
  }
  return sessions;
}
