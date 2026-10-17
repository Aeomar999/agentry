import { useEffect } from "react";
import { Link } from "react-router-dom";
import { X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Banner } from "@/components/molecules/Banner";
import { KeyValueList } from "@/components/molecules/KeyValueList";
import { Mono } from "@/components/molecules/Mono";
import { Panel } from "@/components/molecules/Panel";
import { WorkerStatusBadge } from "@/components/molecules/StatusBadges";
import { AgentName } from "@/components/molecules/AgentName";
import type { MachineAbout, WorkerDetail } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { bytes, LOCAL } from "@/lib/machines";
import { dateTime, timeAgo } from "@/lib/time";

// One machine in detail, for the Workers page's side panel: the system and
// what agents will find on it, read from it now; Agentry on it; its work,
// the agents that may run there, and when it came and went.
export function MachineDetails({ name, onClose }: { name: string; onClose: () => void }) {
  const { data, error, reload } = useResource<WorkerDetail>(`/api/workers/${encodeURIComponent(name)}`);
  // Load and memory change without anything in the log moving: read again now and then.
  useEffect(() => {
    const timer = setInterval(reload, 10_000);
    return () => clearInterval(timer);
  }, [reload]);

  const about = data?.about;
  const title = name === LOCAL ? "This machine" : name;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <h2 className="min-w-0 truncate font-display text-[16px] font-bold text-ink-primary">{title}</h2>
        {data && <WorkerStatusBadge online={data.worker.online} />}
        <IconButton icon={<X />} label="Close" onClick={onClose} className="ml-auto size-7" />
      </div>
      {error && <Banner title="Couldn't load the machine">{error}</Banner>}
      {data?.aboutError && (
        <Banner tone="warning" title="Couldn't read it now">
          {data.aboutError}
        </Banner>
      )}

      {about && <System about={about} />}
      {about && <Tools about={about} />}

      {data && (
        <Panel title="Work here">
          <KeyValueList
            items={[
              { label: "Tasks", value: <TaskLinks tasks={data.worker.tasks} /> },
              ...(about
                ? [
                    { label: "Running", value: <TaskLinks tasks={about.running} /> },
                    { label: "Worktrees", value: <TaskLinks tasks={about.worktrees} /> },
                    ...(name !== LOCAL ? [{ label: "Repos", value: <Mono>{about.repos.join(", ") || "none yet"}</Mono> }] : []),
                  ]
                : []),
              {
                label: "Agents",
                value: data.agents.length ? (
                  <span className="flex flex-wrap gap-x-2">
                    {data.agents.map((agent) => (
                      <Link key={agent} to={`/agents/${agent}`}>
                        <AgentName name={agent} />
                      </Link>
                    ))}
                  </span>
                ) : (
                  <span className="text-ink-muted">none run here; pick it under Runs on</span>
                ),
              },
            ]}
          />
        </Panel>
      )}

      {about && (
        <Panel title="Agentry">
          <KeyValueList
            items={[
              { label: "Version", value: <Mono>{about.agentry || "?"}</Mono> },
              { label: "Binary", value: <Mono>{about.binary}</Mono> },
              { label: "Folder", value: <Mono>{about.folder}</Mono> },
              {
                label: "PATH",
                value: (
                  <span className="flex flex-col">
                    {about.path.map((dir, i) => (
                      <Mono key={`${dir}-${i}`} className="text-ink-secondary">
                        {dir}
                      </Mono>
                    ))}
                  </span>
                ),
              },
            ]}
          />
        </Panel>
      )}

      {data && data.history.length > 0 && (
        <Panel title="History">
          <ol className="flex flex-col gap-1.5 text-[12px]">
            {data.history.map((entry, i) => (
              <li key={`${entry.at}-${i}`} className="flex items-center gap-2">
                <span className={entry.what === "offline" ? "text-ink-muted" : "text-ink-primary"}>{entry.what}</span>
                <Mono className="ml-auto text-ink-faint">{dateTime(entry.at)}</Mono>
              </li>
            ))}
          </ol>
        </Panel>
      )}
    </div>
  );
}

function System({ about }: { about: MachineAbout }) {
  const used = (total: number, free: number) => (total ? `${bytes(total - free)} used of ${bytes(total)}, ${bytes(free)} free` : "unknown");
  return (
    <Panel title="System">
      <KeyValueList
        items={[
          { label: "Host", value: <Mono>{about.host}</Mono> },
          { label: "OS", value: `${about.os} (${about.arch})` },
          { label: "Kernel", value: <Mono>{about.kernel}</Mono> },
          { label: "CPU", value: `${about.cpu || "unknown"}, ${about.cpus} threads` },
          { label: "Memory", value: used(about.memory, about.memoryFree) },
          { label: "Disk", value: used(about.disk, about.diskFree) },
          { label: "Load", value: <Mono>{about.load?.map((l) => l.toFixed(2)).join("  ") ?? "unknown"}</Mono> },
          {
            label: "Up",
            value: about.bootedAt ? `since ${dateTime(about.bootedAt)} (${timeAgo(about.bootedAt).replace(" ago", "")})` : "unknown",
          },
          { label: "Clock", value: `${dateTime(about.time)} ${about.zone}` },
        ]}
      />
    </Panel>
  );
}

function Tools({ about }: { about: MachineAbout }) {
  return (
    <Panel title="What agents find" flush>
      <ul>
        {about.tools.map((tool) => (
          <li key={tool.name} className="flex flex-col gap-0.5 border-b border-divider px-4 py-2 last:border-b-0">
            <div className="flex items-center gap-2 text-[12px]">
              <span className="font-semibold text-ink-primary">{tool.name}</span>
              {tool.path ? (
                <Mono className="ml-auto text-ink-secondary">{tool.version || "installed"}</Mono>
              ) : (
                <span className="ml-auto text-[12px] text-warning">not installed</span>
              )}
            </div>
            {tool.path && <Mono className="truncate text-[11px] text-ink-faint">{tool.path}</Mono>}
          </li>
        ))}
      </ul>
    </Panel>
  );
}

function TaskLinks({ tasks }: { tasks: string[] }) {
  if (tasks.length === 0) return <span className="text-ink-muted">none</span>;
  return (
    <span className="flex flex-wrap gap-x-2">
      {tasks.map((task) => (
        <Link key={task} to={`/tasks/${task}`} className="font-mono text-[12px] text-ink-secondary hover:text-ink-primary">
          {task}
        </Link>
      ))}
    </span>
  );
}
