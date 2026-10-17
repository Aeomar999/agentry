import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { LayoutGrid, List, Network, Plus, Trash2, Users } from "lucide-react";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { PageHeader } from "@/components/molecules/PageHeader";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { RoleBadge } from "@/components/molecules/StatusBadges";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { useToast } from "@/components/organisms/Toasts";
import { api } from "@/api/client";
import { runsOn } from "@/lib/machines";
import { teamOf, teamsOf } from "@/lib/teams";
import type { Agent } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useRowCursor } from "@/lib/shortcuts";
import { cn } from "@/lib/cn";

// The agents, in their teams. A team is a coordinator and the agents it
// hands work to, for one kind of work: Software, Writing, Research, Design,
// or one of your own. Three ways to see them: the teams as cards, each
// team as an org chart (its coordinator over its agents), or one dense list.
// Add a team makes one from a template, every agent with a prompt written
// for that work, which you can then change. A team, or one agent, can be
// removed once no unfinished task needs it.

type View = "teams" | "org" | "list";

interface Template {
  team: string;
  description: string;
  agents: { name: string; description: string; coordinates: boolean }[];
}

const COLUMNS: Column<Agent>[] = [
  { header: "Name", className: "w-40", render: (agent) => <AgentName name={agent.name} className="font-semibold text-ink-primary" /> },
  { header: "Team", className: "w-28", render: (agent) => <span className="text-ink-secondary">{teamOf(agent)}</span> },
  { header: "Role", className: "w-28", render: (agent) => <RoleBadge agent={agent} /> },
  {
    header: "Description",
    className: "w-full max-w-0",
    render: (agent) => <p className="truncate text-ink-secondary">{agent.description}</p>,
  },
  { header: "Engine", className: "w-24 max-md:hidden", render: (agent) => <Mono>{agent.engine}</Mono> },
  { header: "Profile", className: "w-32 max-md:hidden", render: (agent) => <Mono>{agent.profile || "default"}</Mono> },
  {
    header: "Runs on",
    className: "w-36 max-lg:hidden",
    render: (agent) => <span className="text-ink-muted">{runsOn(agent.workers)}</span>,
  },
];

export function AgentsPage() {
  useBreadcrumb({ label: "Agents" });
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const asked = params.get("view") as View | null;
  const view: View = asked === "org" || asked === "list" ? asked : "teams";
  const { data, error, reload } = useResource<Agent[]>("/api/agents");
  const agents = data ?? [];
  const teams = teamsOf(agents);
  const templates = useResource<Template[]>("/api/teams/templates").data ?? [];
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Removing | null>(null);
  const canRemoveTeam = teams.length > 1;
  const open = (agent: Agent) => navigate(`/agents/${encodeURIComponent(agent.name)}`);
  const ordered = teams.flatMap((t) => t.agents);
  const cursor = useRowCursor(view === "list" ? ordered.length : 0, (index) => open(ordered[index]));

  return (
    <>
      <PageHeader
        title="Agents"
        meta="Teams of agents, one for each kind of work. A task goes to its team's coordinator, which hands each step to one of the team's agents."
        actions={[
          { label: "Add a team", icon: <Users />, onClick: () => setAdding(true) },
          { label: "New agent", icon: <Plus />, to: "/agents/new", primary: true },
        ]}
      />
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the agents">{error}</Banner>
        </div>
      )}
      <div className="flex items-center gap-3 border-b border-divider px-6 py-2.5">
        <SegmentedControl
          active={view}
          onChange={(id) => setParams(id === "teams" ? {} : { view: id }, { replace: true })}
          segments={[
            { id: "teams", label: "Teams", icon: <LayoutGrid /> },
            { id: "org", label: "Org chart", icon: <Network /> },
            { id: "list", label: "List", icon: <List /> },
          ]}
        />
        <span className="text-[12px] text-ink-muted">
          {teams.length} {teams.length === 1 ? "team" : "teams"} · {agents.length} agents
        </span>
      </div>

      {view === "list" && (
        <ResourceTable columns={COLUMNS} rows={ordered} rowKey={(agent) => agent.name} onOpen={open} activeKey={ordered[cursor]?.name} />
      )}
      {view === "teams" && (
        <div className="flex flex-col gap-8 p-6">
          {teams.map(({ team, agents }) => (
            <section key={team} className="flex flex-col gap-3">
              <TeamHeading
                team={team}
                count={agents.length}
                description={templates.find((t) => t.team === team)?.description}
                onRemove={canRemoveTeam ? () => setRemoving({ team, agents }) : undefined}
              />
              <div className="grid [grid-template-columns:repeat(auto-fill,minmax(280px,1fr))] gap-3">
                {agents.map((agent) => (
                  <AgentCard key={agent.name} agent={agent} onRemove={() => setRemoving({ agent })} />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
      {view === "org" && (
        <div className="flex flex-col gap-10 overflow-x-auto p-6">
          {teams.map(({ team, agents }) => (
            <section key={team} className="flex flex-col gap-4">
              <TeamHeading team={team} count={agents.length} onRemove={canRemoveTeam ? () => setRemoving({ team, agents }) : undefined} />
              <OrgChart agents={agents} />
            </section>
          ))}
        </div>
      )}

      {removing && (
        <RemoveDialog
          removing={removing}
          onClose={() => setRemoving(null)}
          onRemoved={() => {
            setRemoving(null);
            reload();
          }}
        />
      )}
      {adding && (
        <Dialog title="Add a team" onClose={() => setAdding(false)} wide>
          <AddTeam
            templates={templates}
            existing={teams.map((t) => t.team)}
            onAdded={(team) => {
              setAdding(false);
              reload();
              notify(team);
            }}
          />
        </Dialog>
      )}
    </>
  );

  function notify(team: string) {
    setParams({}, { replace: true });
    document.getElementById(`team-${team}`)?.scrollIntoView({ behavior: "smooth" });
  }
}

interface TeamHeadingProps {
  team: string;
  count: number;
  description?: string;
  onRemove?: () => void; // none for the last team
}

function TeamHeading({ team, count, description, onRemove }: TeamHeadingProps) {
  return (
    <div id={`team-${team}`} className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
      <h2 className="font-display text-[17px] font-bold tracking-tight text-ink-primary [font-stretch:85%]">{team}</h2>
      <span className="text-[12px] text-ink-muted">
        {count} {count === 1 ? "agent" : "agents"}
      </span>
      {description && <span className="text-[12px] text-ink-muted">· {description}</span>}
      <Link to={`/agents/new?team=${encodeURIComponent(team)}`} className="ml-auto text-[12px] text-ink-muted hover:text-ink-primary">
        + Add agent
      </Link>
      {onRemove && (
        <button type="button" onClick={onRemove} className="cursor-pointer text-[12px] text-ink-muted hover:text-danger">
          Remove team
        </button>
      )}
    </div>
  );
}

function AgentCard({ agent, onRemove }: { agent: Agent; onRemove: () => void }) {
  return (
    <Link
      to={`/agents/${encodeURIComponent(agent.name)}`}
      className={cn(
        "group flex flex-col gap-2 rounded-lg border bg-surface-0 p-4 transition-colors duration-(--dur-fast) hover:bg-surface-1",
        agent.coordinates ? "border-divider-2" : "border-divider",
      )}
    >
      <span className="flex items-center gap-2">
        <AgentName name={agent.name} className="text-[13px] font-semibold text-ink-primary" />
        <span className="ml-auto flex gap-1">
          {agent.coordinates && <Badge variant="brand">coordinator</Badge>}
          {agent.column && <Badge>{agent.column}</Badge>}
        </span>
        <button
          type="button"
          aria-label={`Remove ${agent.name}`}
          title={`Remove ${agent.name}`}
          onClick={(event) => (event.preventDefault(), onRemove())}
          className="flex size-5 cursor-pointer items-center justify-center text-ink-faint opacity-0 group-hover:opacity-100 hover:text-danger focus-visible:opacity-100 [&>svg]:size-3.5"
        >
          <Trash2 />
        </button>
      </span>
      <p className="line-clamp-3 text-[12.5px] leading-5 text-ink-secondary">{agent.description}</p>
      <span className="mt-auto flex flex-wrap gap-x-3 gap-y-1 pt-1 font-mono text-[11px] text-ink-faint">
        <span>
          {agent.engine}
          {agent.model ? ` · ${agent.model}` : ""}
        </span>
        <span>{agent.profile || "default"} profile</span>
        <span>{runsOn(agent.workers)}</span>
      </span>
    </Link>
  );
}

// A team as its coordinator over the agents it hands work to, in order.
function OrgChart({ agents }: { agents: Agent[] }) {
  const lead = agents.find((a) => a.coordinates);
  const members = agents.filter((a) => !a.coordinates);
  return (
    <div className="flex min-w-max flex-col items-center">
      {lead ? <Node agent={lead} /> : <span className="text-[12px] text-warning">This team has no coordinator yet.</span>}
      {members.length > 0 && (
        <>
          <span className="h-5 w-px bg-divider-2" aria-hidden />
          <div className="flex">
            {members.map((agent, i) => (
              <div key={agent.name} className="relative flex flex-col items-center px-3 pt-5">
                {/* the line from the lead's line to this agent */}
                <span
                  className={cn(
                    "absolute top-0 h-px bg-divider-2",
                    members.length === 1
                      ? "hidden"
                      : i === 0
                        ? "right-0 left-1/2"
                        : i === members.length - 1
                          ? "right-1/2 left-0"
                          : "right-0 left-0",
                  )}
                  aria-hidden
                />
                <span className="absolute top-0 left-1/2 h-5 w-px bg-divider-2" aria-hidden />
                <Node agent={agent} />
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function Node({ agent }: { agent: Agent }) {
  return (
    <Link
      to={`/agents/${encodeURIComponent(agent.name)}`}
      className={cn(
        "flex w-52 flex-col gap-1 rounded-lg border bg-surface-0 px-3.5 py-2.5 hover:bg-surface-1",
        agent.coordinates ? "border-brand/50" : "border-divider-2",
      )}
    >
      <span className="flex items-center gap-2">
        <AgentName name={agent.name} className="text-[12.5px] font-semibold text-ink-primary" />
        {agent.column && <Badge className="ml-auto">{agent.column}</Badge>}
        {agent.coordinates && (
          <Badge variant="brand" className="ml-auto">
            lead
          </Badge>
        )}
      </span>
      <span className="line-clamp-2 text-[11.5px] leading-4 text-ink-muted">{agent.description}</span>
    </Link>
  );
}

// The ready-made teams, to add one: its agents come with prompts written
// for its kind of work.
function AddTeam({ templates, existing, onAdded }: { templates: Template[]; existing: string[]; onAdded: (team: string) => void }) {
  const notify = useToast();
  const [busy, setBusy] = useState("");
  async function add(template: Template) {
    setBusy(template.team);
    try {
      await api("POST", "/api/teams", { template: template.team });
      notify("success", `Added the ${template.team} team`, "Pick it on New task; its coordinator takes the task.");
      onAdded(template.team);
    } catch (failure) {
      notify("danger", "The team wasn't added", (failure as Error).message);
      setBusy("");
    }
  }
  return (
    <div className="flex flex-col gap-3">
      <p className="text-[12px] text-ink-muted">
        Each team comes with a coordinator and its specialists, each with a prompt written for the work. Change any of them afterwards, or
        add agents of your own with New agent.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {templates.map((template) => {
          const has = existing.includes(template.team);
          return (
            <div key={template.team} className="flex flex-col gap-2 rounded-lg border border-divider bg-surface-0 p-4">
              <span className="flex items-center gap-2">
                <span className="text-[14px] font-semibold text-ink-primary">{template.team}</span>
                {has && <Badge>you have it</Badge>}
              </span>
              <p className="text-[12px] leading-5 text-ink-secondary">{template.description}</p>
              <span className="flex flex-wrap gap-1.5">
                {template.agents.map((a) => (
                  <span
                    key={a.name}
                    title={a.description}
                    className={cn(
                      "rounded-sm px-1.5 font-mono text-[11px] leading-5",
                      a.coordinates ? "bg-brand-soft text-brand" : "bg-surface-3 text-ink-secondary",
                    )}
                  >
                    {a.name}
                  </span>
                ))}
              </span>
              <span className="mt-auto pt-1">
                <Button size="sm" variant={has ? "ghost" : "primary"} disabled={Boolean(busy)} onClick={() => add(template)}>
                  {busy === template.team ? "Adding…" : has ? "Add another" : "Add this team"}
                </Button>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

type Removing = { team: string; agents: Agent[] } | { agent: Agent };

// RemoveDialog removes a team with all its agents, or one agent, after
// saying what goes. Agentry refuses while an unfinished task needs them,
// and says which.
function RemoveDialog({ removing, onClose, onRemoved }: { removing: Removing; onClose: () => void; onRemoved: () => void }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState("");
  const isTeam = "team" in removing;
  const name = isTeam ? `the ${removing.team} team` : removing.agent.name;
  const lonelyLead = !isTeam && removing.agent.coordinates;

  async function remove() {
    setBusy(true);
    setRefused("");
    try {
      if (isTeam) await api("DELETE", `/api/teams/${encodeURIComponent(removing.team)}`);
      else await api("DELETE", `/api/agents/${encodeURIComponent(removing.agent.name)}`);
      toast("success", `Removed ${name}`);
      onRemoved();
    } catch (failure) {
      setRefused((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <Dialog title={`Remove ${name}?`} onClose={onClose}>
      <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
        {isTeam ? (
          <p>
            Its {removing.agents.length} agents go with it: {removing.agents.map((a) => a.name).join(", ")}. Their prompts and what they
            learned go too. Finished tasks keep their history.
          </p>
        ) : (
          <p>
            Its prompt and what it learned go with it. Finished tasks keep their history.
            {lonelyLead && " A coordinator goes only with its team, unless another agent of the team coordinates first."}
          </p>
        )}
        {refused && <Banner title="It wasn't removed">{refused}</Banner>}
        <div className="flex gap-2">
          <Button variant="danger" icon={<Trash2 />} disabled={busy} onClick={remove}>
            Remove
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Keep
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
