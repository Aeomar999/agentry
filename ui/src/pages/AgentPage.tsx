import { useParams } from "react-router-dom";
import { Bot, Pencil } from "lucide-react";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { CodeViewer } from "@/components/molecules/CodeViewer";
import { KeyValueList } from "@/components/molecules/KeyValueList";
import { RoleBadge } from "@/components/molecules/StatusBadges";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import type { Agent } from "@/api/types";
import { useResource } from "@/live/use-resource";

export function AgentPage() {
  const { name = "" } = useParams();
  useBreadcrumb({ label: "Agents", to: "/agents" }, { label: name });
  const { data } = useResource<Agent[]>("/api/agents");
  if (!data) return null;
  const agent = data.find((a) => a.name === name);
  if (!agent) return <EmptyState icon={<Bot />} title={`No agent named ${name}`} />;

  return (
    <>
      <PageHeader
        title={<AgentName name={agent.name} />}
        badges={<RoleBadge agent={agent} />}
        meta={agent.description}
        actions={[{ label: "Edit", icon: <Pencil />, to: `/agents/${encodeURIComponent(agent.name)}/edit` }]}
      />
      <div className="flex flex-col gap-6 p-6 xl:flex-row xl:items-start">
        <div className="min-w-0 flex-1">
          <Panel flush title="Prompt" aside="Its standing instructions, given at the start of every turn">
            <CodeViewer text={agent.prompt} language="markdown" maxHeight={2000} />
          </Panel>
        </div>
        <aside className="w-full shrink-0 xl:w-inspector">
          <Panel title="Settings">
            <KeyValueList
              items={[
                { label: "Engine", value: <Mono>{agent.engine}</Mono> },
                { label: "Model", value: <Mono>{agent.model || "the engine's default"}</Mono> },
                { label: "Policy profile", value: <Mono>{agent.profile || "the default profile"}</Mono> },
                { label: "Role", value: agent.coordinates ? "Decides who works next" : "Works on the task" },
                { label: "Runs on", value: agent.workers.length ? <Mono>{agent.workers.join(", ")}</Mono> : "This machine" },
              ]}
            />
          </Panel>
        </aside>
      </div>
    </>
  );
}
