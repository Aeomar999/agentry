import { Badge, BadgeVariant } from "@/components/atoms/Badge";
import type { Action, Agent, Decision, TaskStatus } from "@/api/types";

// The domain's states, each with one colour everywhere it appears:
// working is info, needs you is warning, done and allow are success,
// deny is danger.

const TASK: Record<TaskStatus, BadgeVariant> = { working: "info", "needs you": "warning", done: "success", held: "default" };

export function TaskStatusBadge({ status }: { status: TaskStatus }) {
  return <Badge variant={TASK[status]}>{status}</Badge>;
}

// A task that wants a turn and waits for a slot, as the most tasks running
// at once are.
export function QueuedBadge() {
  return <Badge>queued</Badge>;
}

const DECISION: Record<Decision, BadgeVariant> = { allow: "success", ask: "warning", deny: "danger" };

export function DecisionBadge({ decision }: { decision: Decision }) {
  return <Badge variant={DECISION[decision]}>{decision}</Badge>;
}

// An action's state: decided, waiting for you, or still being judged.
export function ActionBadge({ action }: { action: Action }) {
  if (action.decision === "approved") return <Badge variant="success">approved</Badge>;
  if (action.decision === "denied") return <Badge variant="danger">denied</Badge>;
  return action.asked ? <Badge variant="warning">waiting for you</Badge> : <Badge>deciding</Badge>;
}

export function WorkerStatusBadge({ online }: { online: boolean }) {
  return online ? <Badge variant="success">online</Badge> : <Badge>offline</Badge>;
}

export function RoleBadge({ agent }: { agent: Agent }) {
  return agent.coordinates ? <Badge variant="brand">coordinator</Badge> : <Badge>worker</Badge>;
}
