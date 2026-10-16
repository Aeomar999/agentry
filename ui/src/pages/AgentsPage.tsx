import { useNavigate } from "react-router-dom";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { RoleBadge } from "@/components/molecules/StatusBadges";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import type { Agent } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useRowCursor } from "@/lib/shortcuts";

const COLUMNS: Column<Agent>[] = [
  { header: "Name", className: "w-32", render: (agent) => <Mono className="font-semibold text-ink-primary">{agent.name}</Mono> },
  { header: "Role", className: "w-28", render: (agent) => <RoleBadge agent={agent} /> },
  {
    header: "Description",
    className: "w-full max-w-0",
    render: (agent) => <p className="truncate text-ink-secondary">{agent.description}</p>,
  },
  { header: "Engine", className: "w-24 max-md:hidden", render: (agent) => <Mono>{agent.engine}</Mono> },
  {
    header: "Model",
    className: "w-28 max-lg:hidden",
    render: (agent) => <Mono className="text-ink-muted">{agent.model || "default"}</Mono>,
  },
  { header: "Profile", className: "w-32 max-md:hidden", render: (agent) => <Mono>{agent.profile || "default"}</Mono> },
];

export function AgentsPage() {
  useBreadcrumb({ label: "Agents" });
  const navigate = useNavigate();
  const { data, error } = useResource<Agent[]>("/api/agents");
  const agents = data ?? [];
  const open = (agent: Agent) => navigate(`/agents/${encodeURIComponent(agent.name)}`);
  const cursor = useRowCursor(agents.length, (index) => open(agents[index]));

  return (
    <>
      <PageHeader title="Agents" meta="The coordinator chooses from these for every step. Each starts from its own prompt." />
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the agents">{error}</Banner>
        </div>
      )}
      <ResourceTable columns={COLUMNS} rows={agents} rowKey={(agent) => agent.name} onOpen={open} activeKey={agents[cursor]?.name} />
    </>
  );
}
