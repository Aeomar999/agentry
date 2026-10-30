import { useEffect, useRef, useState } from "react";
import { Plug, TerminalSquare, Trash2 } from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { Banner } from "@/components/molecules/Banner";
import { InstallCommand } from "@/components/molecules/InstallCommand";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Mono } from "@/components/molecules/Mono";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { Button } from "@/components/atoms/Button";
import { useToast } from "@/shell/Toasts";
import { api } from "@/api/client";
import type { InstallCommands } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { clockTime, dateTime } from "@/lib/time";

// Integrations connect Agentry to the tools you work in. Claude Code first:
// one command, run on any machine, gives its Claude Code Agentry's tools in
// every repo (agentry mcp), with its status line and notifications. Each
// machine connected is listed, and can be revoked.

interface Connection {
  id: string;
  host: string;
  user: string;
  os: string;
  madeAt: string;
}

const OS_NAMES: Record<string, string> = { darwin: "macOS", linux: "Linux", windows: "Windows" };

export function IntegrationsPage() {
  useBreadcrumb({ label: "Integrations" });
  const { data, reload } = useResource<{ connections: Connection[] }>("/api/integrations/connections");
  const connections = data?.connections ?? [];

  return (
    <>
      <PageHeader title="Integrations" meta="Connect Agentry to the tools you work in, so you can hand it work and hear from it there." />
      <div className="flex max-w-5xl flex-col gap-6 p-6">
        <ClaudeCode connections={connections} onChanged={reload} />
      </div>
    </>
  );
}

function ClaudeCode({ connections, onChanged }: { connections: Connection[]; onChanged: () => void }) {
  const notify = useToast();
  const [invite, setInvite] = useState<InstallCommands | null>(null);
  const [error, setError] = useState("");
  const before = useRef<Set<string> | null>(null);
  const joined = invite && before.current ? connections.find((c) => !before.current!.has(c.id)) : undefined;

  useEffect(() => {
    if (joined) onChanged();
  }, [joined, onChanged]);

  async function makeCommand() {
    setError("");
    before.current = new Set(connections.map((c) => c.id));
    try {
      setInvite(await api("POST", "/api/integrations/claude-code"));
    } catch (failure) {
      setError((failure as Error).message);
    }
  }

  async function revoke(connection: Connection) {
    try {
      await api("DELETE", `/api/integrations/connections/${encodeURIComponent(connection.id)}`);
      notify("success", `Disconnected ${connection.host}`, "Its Claude Code can't reach Agentry any more.");
      onChanged();
    } catch (failure) {
      notify("danger", "It is still connected", (failure as Error).message);
    }
  }

  const columns: Column<Connection>[] = [
    { header: "Machine", className: "w-full max-w-0", render: (c) => <span className="font-medium text-ink-primary">{c.host}</span> },
    { header: "User", className: "w-32", render: (c) => <Mono>{c.user || "?"}</Mono> },
    { header: "System", className: "w-28", render: (c) => <span className="text-ink-secondary">{OS_NAMES[c.os] ?? c.os}</span> },
    { header: "Connected", className: "w-44 max-md:hidden", render: (c) => <span className="text-ink-muted">{dateTime(c.madeAt)}</span> },
    {
      header: "",
      className: "w-24 text-right",
      render: (c) => (
        <button
          type="button"
          onClick={(event) => (event.stopPropagation(), revoke(c))}
          className="inline-flex cursor-pointer items-center gap-1 text-[12px] text-ink-muted hover:text-danger [&>svg]:size-3.5"
        >
          <Trash2 />
          Revoke
        </button>
      ),
    },
  ];

  return (
    <Panel title={<>Claude Code</>} aside={<span>{connections.length} connected</span>}>
      <div className="flex flex-col gap-5 text-[13px] text-ink-secondary">
        <div className="grid gap-5 md:grid-cols-[1fr_1fr]">
          <div className="flex flex-col gap-2">
            <p className="text-ink-primary">
              Work with Agentry from plain <code className="font-mono">claude</code>, in any repo.
            </p>
            <ul className="flex list-disc flex-col gap-1 pl-5 marker:text-ink-faint">
              <li>Hand it work: tasks and epics in the repo you're in, filed, ordered and started.</li>
              <li>Tell a coordinator something, or answer its question.</li>
              <li>Approve actions and close epics: Claude Code asks you directly, so the model can't approve for you.</li>
              <li>A status line with what needs you, and a notification when something new does.</li>
            </ul>
            <p className="text-[12px] text-ink-muted">Everything it does is on this board at once.</p>
          </div>
          <div className="flex flex-col gap-3">
            {error && <Banner title="Couldn't make an install command">{error}</Banner>}
            {joined ? (
              <Banner tone="success" title={`${joined.host} is connected`}>
                Start <code className="font-mono">claude</code> in any repo there and ask it for your Agentry board.
              </Banner>
            ) : invite ? (
              <>
                <p>Run this in a terminal on the machine. It installs agentry, connects it and sets up Claude Code.</p>
                <InstallCommand commands={invite} />
                <p className="text-[12px] text-ink-muted">
                  It works once, until {clockTime(invite.expires)}, and holds no password: the machine gets its own credential, which you
                  can revoke here.
                </p>
              </>
            ) : (
              <>
                <p>Make a one-time command, then run it on the machine you use Claude Code on.</p>
                <div>
                  <Button variant="primary" icon={<TerminalSquare />} onClick={makeCommand}>
                    Make an install command
                  </Button>
                </div>
                <p className="text-[12px] text-ink-muted">
                  On the machine Agentry runs on, <code className="font-mono text-ink-secondary">agentry mcp install</code> does the same.
                </p>
              </>
            )}
          </div>
        </div>
        {connections.length > 0 ? (
          <div className="overflow-hidden rounded-md border border-divider">
            <ResourceTable columns={columns} rows={connections} rowKey={(c) => c.id} />
          </div>
        ) : (
          <EmptyState icon={<Plug />} title="No machine connected yet">
            Make an install command and run it where you use Claude Code.
          </EmptyState>
        )}
      </div>
    </Panel>
  );
}
