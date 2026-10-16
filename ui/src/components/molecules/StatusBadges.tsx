import { Badge, BadgeVariant } from "@/components/atoms/Badge";
import type { Action, Agent, Decision, TaskStatus } from "@/api/types";

// The domain's states, each with one colour everywhere it appears:
// working is info, needs you is warning, done and allow are success,
// deny is danger.

const TASK: Record<TaskStatus, BadgeVariant> = { working: "info", "needs you": "warning", done: "success", held: "default" };

export function TaskStatusBadge({ status }: { status: TaskStatus }) {
  return <Badge variant={TASK[status]}>{status}</Badge>;
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

export function RoleBadge({ agent }: { agent: Agent }) {
  return agent.coordinates ? <Badge variant="brand">coordinator</Badge> : <Badge>worker</Badge>;
}
