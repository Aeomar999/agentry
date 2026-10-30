import { useParams } from "react-router-dom";
import { Bot } from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { EmptyState } from "@/components/molecules/EmptyState";
import { AgentForm } from "@/domains/agents/AgentForm";
import type { Agent } from "@/api/types";
import { useResource } from "@/live/use-resource";

// /agents/new adds an agent; /agents/:name/edit changes one.
export function AgentEditPage() {
  const { name } = useParams();
  useBreadcrumb(
    { label: "Agents", to: "/agents" },
    ...(name ? [{ label: name, to: `/agents/${name}` }, { label: "Edit" }] : [{ label: "New agent" }]),
  );
  const { data } = useResource<Agent[]>(name ? "/api/agents" : null);
  if (name && !data) return null;
  const agent = name ? data?.find((a) => a.name === name) : undefined;
  if (name && !agent) return <EmptyState icon={<Bot />} title={`No agent named ${name}`} />;

  return (
    <>
      <PageHeader
        title={agent ? <span className="font-mono">{agent.name}</span> : "New agent"}
        meta={agent ? "Changes apply from the agent's next turn." : "The coordinator can hand work to it from its next turn."}
      />
      <div className="p-6">
        {/* keyed by name, so the form starts again for another agent */}
        <AgentForm key={agent?.name ?? "new"} agent={agent} />
      </div>
    </>
  );
}
