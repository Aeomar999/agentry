// The JSON `agentry serve` answers with, as defined in internal/web/views.go.

export type TaskStatus = "working" | "needs you" | "done" | "held";

export interface TaskSummary {
  id: string;
  title: string;
  repo: string;
  status: TaskStatus;
  currentAgent: string;
  stage: string; // the agent the coordinator last handed it to: its column on the board
  turns: number;
  needsYou: string;
  hasQuestion: boolean; // the coordinator asked you something: what you send next is the answer
  labels: string[];
  budget: number; // tokens; 0 is no limit
  tokensUsed: number;
  overBudget: boolean;
  rank: number; // your priority order, lower first
  parked: "" | "todo" | "backlog"; // while you hold it there
  createdAt: string;
  lastActiveAt: string;
}

export interface TaskDetail extends TaskSummary {
  description: string;
  firstTitle: string; // the title it was created with, when the coordinator renamed it
  options: string[]; // answers the open question offers
  page: string; // the page the open question came with
  coordinator: string;
  summary: string;
  worktree: Worktree | null;
  trail: Step[] | null;
  turnList: Turn[] | null;
  approvals: Action[] | null;
  actions: Action[] | null;
  events: LogEvent[] | null;
}

export interface Worktree {
  path: string;
  branch: string;
  base: string;
  port: number;
}

export interface Step {
  at: string;
  from: string;
  to: string;
  text: string;
}

export interface Turn {
  id: string;
  agent: string;
  startedAt: string;
  acknowledged: boolean;
  ended: boolean;
  issue: string;
  tokens: number;
}

export interface Action {
  id: string;
  turn: string;
  agent: string;
  tool: string;
  command: string;
  requestedAt: string;
  asked: boolean;
  decision: string;
  reason: string;
}

export interface LogEvent {
  position: number;
  at: string;
  type: string;
  actor: string;
  correlation?: string;
  data?: Record<string, unknown>;
}

export interface Agent {
  name: string;
  description: string;
  prompt: string;
  engine: string;
  model: string;
  profile: string;
  coordinates: boolean;
  workers: string[]; // the machines it may run on; none: the machine Agentry runs on
  color: string; // one of the agent colours; "" for the next free one
  column: "" | "review"; // where its work shows on the board: In progress, or Review
}

export type Decision = "allow" | "ask" | "deny";

export interface Policy {
  path: string;
  hash: string;
  problem: string;
  defaultProfile: string;
  useForEveryone: string;
  profiles: PolicyProfile[];
  text: string;
}

export interface PolicyProfile {
  name: string;
  why: string;
  otherwise: Decision;
  rules: { name: string; decision: Decision; why: string }[];
}

export interface ServerConfig {
  defaultRepo: string;
  budget: number; // what a new task gets, in tokens
  hasTerminal: boolean;
  hasDesk: boolean;
  engines: string[];
  workers: string[]; // the machines agents can be given; none yet
}

export interface CreatedEvent {
  stream: string;
}

// Something waiting for you, from any task (internal/web/questions_api.go).
export interface Question {
  id: string;
  kind: "approval" | "question" | "budget" | "waiting";
  task: string;
  taskTitle: string;
  agent: string;
  at: string;
  text: string;
  context: string;
  action?: string;
  tool?: string;
  reason?: string;
  options?: string[];
  page?: string;
  tokensUsed?: number;
  budget?: number;
}
