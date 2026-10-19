import { ReactNode } from "react";
import type { Agent } from "@/api/types";
import { roleOf, teamOf } from "@/lib/teams";
import { Avatar } from "./Avatar";

// Who wrote a run of rows in a task's Activity, in text: the chip, the name,
// then the team and role, muted, and after them what the caller adds (a
// turn's facts). An author the roster doesn't know (you, Agentry, an agent
// since removed) has no team or role: there is nothing true to say. Below
// md only the chip and the name show; authorName says the rest.

export function AuthorBand({ name, agent, children }: { name: string; agent?: Agent; children?: ReactNode }) {
  const role = agent ? roleOf(agent) : "";
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2.5 whitespace-nowrap">
      <Avatar name={name} size={20} />
      <span className="max-w-[120px] shrink-0 truncate font-mono text-[12.5px] font-semibold text-ink-primary md:max-w-none">{name}</span>
      {agent && (
        <span className="hidden shrink-0 text-[12px] text-ink-muted md:inline">
          {teamOf(agent)}
          {role && ` · ${role}`}
        </span>
      )}
      {children}
    </span>
  );
}

// authorName is the author as a screen reader hears it, commas for the
// band's middle dot: "release, Software team, deployer".
export function authorName(name: string, agent?: Agent): string {
  if (!agent) return name;
  return [name, `${teamOf(agent)} team`, roleOf(agent)].filter(Boolean).join(", ");
}
