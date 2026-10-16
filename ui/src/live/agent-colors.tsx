import { createContext, ReactNode, useContext, useMemo } from "react";
import type { Agent } from "@/api/types";

// Each agent's colour, as a CSS value. An agent without one gets the next
// colour nobody has, in roster order, so colours stay apart and steady.

export const AGENT_COLORS = ["blue", "violet", "teal", "green", "amber", "orange", "red", "pink", "slate"];

type ColorOf = (agent: string) => string;

const Colors = createContext<ColorOf>(() => "var(--color-ink-faint)");

export function AgentColorsProvider({ agents, children }: { agents: Agent[]; children: ReactNode }) {
  const colorOf = useMemo<ColorOf>(() => {
    const taken = new Set(agents.map((a) => a.color).filter(Boolean));
    const free = AGENT_COLORS.filter((c) => !taken.has(c));
    const byName = new Map<string, string>();
    let next = 0;
    for (const agent of agents) {
      const color = agent.color || free[next++ % Math.max(free.length, 1)] || AGENT_COLORS[0];
      byName.set(agent.name, `var(--color-agent-${color})`);
    }
    // "you" and Agentry's own loop are people and plumbing, not agents.
    return (name) => byName.get(name) ?? "var(--color-ink-faint)";
  }, [agents]);
  return <Colors.Provider value={colorOf}>{children}</Colors.Provider>;
}

export function useAgentColor(): ColorOf {
  return useContext(Colors);
}
