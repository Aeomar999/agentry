import type { Turn } from "@/api/types";

// An agent's plan as it writes it: a Markdown checklist, "- [ ] step" and
// "- [x] step", with any other line (a heading, a note) kept as text.

export interface PlanLine {
  text: string;
  step: boolean; // a checklist step; otherwise a heading or a note
  done: boolean;
  depth: number; // how far it is indented under the step above
}

export interface Plan {
  lines: PlanLine[];
  steps: number;
  done: number;
}

const STEP = /^(\s*)(?:[-*+]|\d+[.)])\s+\[([ xX])\]\s+(.*)$/;
const BULLET = /^(\s*)(?:[-*+]|\d+[.)])\s+(.*)$/;

export function readPlan(text: string): Plan {
  const lines: PlanLine[] = [];
  for (const raw of text.split("\n")) {
    if (!raw.trim()) continue;
    const step = raw.match(STEP);
    if (step) {
      lines.push({ text: step[3].trim(), step: true, done: step[2] !== " ", depth: depthOf(step[1]) });
      continue;
    }
    const bullet = raw.match(BULLET);
    const text = (bullet ? bullet[2] : raw).replace(/^#+\s*/, "").trim();
    lines.push({ text, step: false, done: false, depth: bullet ? depthOf(bullet[1]) : 0 });
  }
  const steps = lines.filter((line) => line.step);
  return { lines, steps: steps.length, done: steps.filter((line) => line.done).length };
}

// Two spaces, or a tab, for each level.
function depthOf(indent: string): number {
  return Math.min(3, Math.floor(indent.replace(/\t/g, "  ").length / 2));
}

// How the plan column groups a task's plans. Each agent has one plan that
// carries on across its turns until it is finished: the coordinator's is the
// task's, pinned at the top. A plan still to be continued stays in the
// column, open while its agent works and closed while it doesn't. Earlier
// keeps only plans that are over: finished, or replaced by a new one.

export interface PlanEntry {
  turn: Turn; // the turn whose plan this is
  plan: Plan;
  agent: string;
  open: boolean; // shown open: its agent is working now
  live: boolean; // its agent is working on it right now
  note: string; // why it is where it is, "" when that is plain
}

export interface PlanSections {
  task?: PlanEntry; // the coordinator's plan for the whole task
  current: PlanEntry[]; // the other agents' plans still to be finished, the working one first
  earlier: PlanEntry[]; // finished or replaced, newest first
  writing?: Turn; // a working turn with no plan yet, and none to pick up
}

export function planSections(turns: Turn[], coordinator: string): PlanSections {
  const working = turns.find((t) => !t.ended);
  const byId = new Map(turns.map((t) => [t.id, t]));
  const planned = turns.filter((t) => t.plan);

  // A plan a later turn took up and kept steps of lives on in that turn; one
  // it took up and threw away was replaced.
  const continuedBy = new Map<string, Turn>();
  for (const later of planned) {
    const earlier = later.planFrom ? byId.get(later.planFrom) : undefined;
    if (earlier?.plan) continuedBy.set(earlier.id, later);
  }
  const kept = (earlier: Turn, later: Turn) => {
    const steps = new Set(
      readPlan(earlier.plan)
        .lines.filter((l) => l.step)
        .map((l) => l.text),
    );
    return readPlan(later.plan).lines.some((l) => l.step && steps.has(l.text));
  };

  const sections: PlanSections = { current: [], earlier: [] };
  const latest = new Map<string, Turn>(); // each agent's newest plan
  for (const turn of planned) latest.set(turn.agent, turn);

  for (const turn of [...planned].reverse()) {
    const plan = readPlan(turn.plan);
    // Worked on now: its own turn is running, or the running turn took it up.
    const pickedUp = Boolean(working && working.planFrom === turn.id && !working.plan);
    const isWorking = working?.id === turn.id || pickedUp;
    const entry: PlanEntry = { turn, plan, agent: turn.agent, open: isWorking, live: isWorking, note: "" };
    const later = continuedBy.get(turn.id);
    if (later && kept(turn, later)) continue; // its steps go on in a later turn
    if (later) {
      sections.earlier.push({ ...entry, open: false, live: false, note: `replaced in ${later.id}` });
      continue;
    }
    if (latest.get(turn.agent) !== turn) {
      if (turn.agent === coordinator) continue; // an older version of the one task plan
      const note = plan.done === plan.steps ? "finished" : turn.issue || turn.stopped ? `stopped at ${plan.done}/${plan.steps}` : "";
      sections.earlier.push({ ...entry, open: false, live: false, note });
      continue;
    }
    if (pickedUp) entry.note = `${working!.id} is picking it up`;
    else if (!isWorking && turn.stopped) entry.note = `you stopped it at ${plan.done}/${plan.steps}`;
    else if (!isWorking && turn.issue) entry.note = `stopped at ${plan.done}/${plan.steps}`; // the turn's issue says why
    if (turn.agent === coordinator) {
      sections.task = { ...entry, open: isWorking || !working };
    } else if (plan.steps > 0 && plan.done === plan.steps && !isWorking) {
      sections.earlier.push({ ...entry, note: "finished" });
    } else {
      sections.current.push(entry);
    }
  }
  sections.current.sort((a, b) => Number(b.open) - Number(a.open));
  if (working && !working.plan && !working.planFrom) sections.writing = working;
  return sections;
}
