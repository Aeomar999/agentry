import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Plus, Server, Trash2 } from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { WorkerStatusBadge } from "@/components/molecules/StatusBadges";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Dialog } from "@/components/molecules/Dialog";
import { AddWorkerDialog } from "@/components/organisms/AddWorkerDialog";
import { MachineDetails } from "@/components/organisms/MachineDetails";
import { useToast } from "@/shell/Toasts";
import { Button } from "@/components/atoms/Button";
import { IconButton } from "@/components/atoms/IconButton";
import { api } from "@/api/client";
import type { ServerConfig, Worker, WorkerList } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { timeAgo } from "@/lib/time";
import { LOCAL } from "@/lib/machines";

// A row is a worker, or this machine, which is always first and always on.
type Row = { machine: Worker; isLocal: boolean };

function columns(onRemove: (worker: Worker) => void): Column<Row>[] {
  return [
    {
      header: "Machine",
      className: "w-44",
      render: ({ machine, isLocal }) => <span className="font-semibold text-ink-primary">{isLocal ? "This machine" : machine.name}</span>,
    },
    { header: "Status", className: "w-24", render: ({ machine }) => <WorkerStatusBadge online={machine.online} /> },
    {
      header: "Host",
      className: "w-48 max-md:hidden",
      render: ({ machine }) => (
        <Mono className="text-ink-muted">
          {machine.host || "?"} {machine.os && `${machine.os}/${machine.arch}`}
        </Mono>
      ),
    },
    {
      header: "Has",
      className: "w-full max-w-0",
      render: ({ machine }) => (
        <p className="truncate">
          <Mono className="text-ink-secondary">{machine.engines.join(", ") || "no engines found"}</Mono>
          {machine.missing.length > 0 && <span className="ml-2 text-[12px] text-warning">no {machine.missing.join(", ")}</span>}
        </p>
      ),
    },
    {
      header: "Work here",
      className: "w-40 max-lg:hidden",
      render: ({ machine }) =>
        machine.tasks.length === 0 ? (
          <span className="text-ink-faint">none</span>
        ) : (
          <span className="flex flex-wrap gap-x-2">
            {machine.tasks.map((task) => (
              <Link key={task} to={`/tasks/${task}`} className="font-mono text-[12px] text-ink-secondary hover:text-ink-primary">
                {task}
              </Link>
            ))}
          </span>
        ),
    },
    {
      header: "Since",
      className: "w-28 max-md:hidden",
      render: ({ machine, isLocal }) =>
        isLocal ? null : (
          <span title={machine.online ? "online since then" : "offline since then"}>
            <Mono className="text-ink-muted">{timeAgo(machine.since)}</Mono>
          </span>
        ),
    },
    {
      header: "",
      className: "w-12",
      render: ({ machine, isLocal }) =>
        isLocal ? null : (
          <IconButton
            icon={<Trash2 />}
            label={`Remove ${machine.name}`}
            onClick={(event) => {
              event.stopPropagation(); // not a click on the row
              onRemove(machine);
            }}
          />
        ),
    },
  ];
}

export function WorkersPage() {
  useBreadcrumb({ label: "Workers" });
  const notify = useToast();
  const { data, error } = useResource<WorkerList>("/api/workers");
  const config = useResource<ServerConfig>("/api/config").data;
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Worker | null>(null);
  const [removeError, setRemoveError] = useState("");
  const workers = data?.workers ?? [];
  const facts = data?.local.machine;
  const here: Worker = {
    name: LOCAL,
    host: facts?.host ?? "",
    os: facts?.os ?? "",
    arch: facts?.arch ?? "",
    joinedAt: "",
    online: true,
    since: "",
    version: facts?.version ?? "",
    engines: facts?.engines ?? [],
    missing: facts?.missing ?? [],
    tasks: data?.local.tasks ?? [],
  };
  // The machine open in the side panel, kept in the address.
  const [params, setParams] = useSearchParams();
  const selected = params.get("machine");
  const select = (name: string | null) => setParams(name ? { machine: name } : {}, { replace: true });
  const rows: Row[] = [{ machine: here, isLocal: true }, ...workers.map((machine) => ({ machine, isLocal: false }))];

  async function remove(worker: Worker) {
    try {
      await api("DELETE", `/api/workers/${encodeURIComponent(worker.name)}`);
      notify("success", `Removed ${worker.name}`);
      setRemoving(null);
    } catch (failure) {
      setRemoveError((failure as Error).message);
    }
  }

  return (
    <>
      <PageHeader
        title="Workers"
        meta="The machines agents run on. An agent with none picked runs on this one; give it workers in its settings."
        actions={
          config?.canAddWorkers === false ? [] : [{ label: "Add worker", icon: <Plus />, onClick: () => setAdding(true), primary: true }]
        }
      />
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the workers">{error}</Banner>
        </div>
      )}
      <div className="flex flex-col xl:flex-row xl:items-start">
        <div className="min-w-0 flex-1">
          <ResourceTable
            columns={columns((w) => (setRemoveError(""), setRemoving(w)))}
            rows={rows}
            rowKey={(row) => row.machine.name}
            onOpen={(row) => select(row.machine.name === selected ? null : row.machine.name)}
            activeKey={selected ?? undefined}
          />
          {data && workers.length === 0 && (
            <EmptyState icon={<Server />} title="No other machines yet">
              Add a worker to run agents on another machine that has Claude Code. A task's work moves there over git when an agent that runs
              there takes it, and comes back when the next one doesn't.
            </EmptyState>
          )}
          {!selected && data && <p className="px-4 py-3 text-[12px] text-ink-muted">Click a machine for its details.</p>}
        </div>
        {selected && (
          <aside className="w-full shrink-0 border-divider p-4 max-xl:border-t xl:w-inspector xl:border-l">
            <MachineDetails key={selected} name={selected} onClose={() => select(null)} />
          </aside>
        )}
      </div>

      {adding && <AddWorkerDialog workers={workers} onClose={() => setAdding(false)} />}
      {removing && (
        <Dialog title={`Remove ${removing.name}?`} onClose={() => setRemoving(null)}>
          <div className="flex flex-col gap-4 text-[13px] text-ink-secondary">
            {removeError && <Banner title="It wasn't removed">{removeError}</Banner>}
            <p>
              Its credential stops working and its link drops. To bring it back, add it again.
              {removing.tasks.length > 0 && " Work on it stays there until it is back."}
            </p>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setRemoving(null)}>
                Cancel
              </Button>
              <Button variant="danger" icon={<Trash2 />} onClick={() => remove(removing)}>
                Remove
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
