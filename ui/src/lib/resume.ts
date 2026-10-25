import type { Agent } from "@/api/types";

// Whether an agent's turns go on with its session from its last turn on the
// task, or each start fresh (internal/agents Agent.Resume). Unset, it goes
// by the agent's column, as the server decides: a reviewer starts fresh and
// is briefed with a compact handoff of the change; every other agent goes
// on with its session, which costs fewer tokens.

export type Resume = Agent["resume"];

export const RESUMES: Resume[] = ["", "resume", "fresh"];

// The settings as the forms name them.
export const RESUME_TITLES: Record<Resume, string> = {
  "": "By its work",
  resume: "Going on with its session",
  fresh: "Fresh",
};

const WORK: Record<Agent["column"], string> = { "": "building", review: "review", ship: "shipping" };

// startsFresh says whether the agent's turns each start a new session.
export function startsFresh({ column, resume }: Pick<Agent, "column" | "resume">): boolean {
  return resume === "fresh" || (resume === "" && column === "review");
}

// startsEachTurn is how the agent starts each turn, for its page: fresh or
// going on, and why when that is set or a reviewer's default.
export function startsEachTurn(agent: Pick<Agent, "column" | "resume">): string {
  if (agent.resume === "fresh") return "Fresh (set to start fresh)";
  if (agent.resume === "resume") return "Going on with its session (set to)";
  return startsFresh(agent) ? "Fresh (reviewers start fresh)" : "Going on with its session";
}

// byItsWork is what By its work comes to for this agent: how it starts, and
// the work that decides it. A coordinator's column isn't shown, so it is
// told by whether it reviews.
export function byItsWork(agent: Pick<Agent, "column" | "resume" | "coordinates">): string {
  const fresh = startsFresh({ column: agent.column, resume: "" });
  const how = fresh ? "Fresh" : "Going on with its session";
  if (agent.coordinates) return `${how}, since it ${fresh ? "reviews" : "doesn't review"}.`;
  return `${how}, since its work counts as ${WORK[agent.column]}.`;
}
