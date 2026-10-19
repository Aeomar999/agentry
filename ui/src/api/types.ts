import type { SharedFile } from "@/lib/shared";
// The JSON `agentry serve` answers with, as defined in internal/web/views.go.

export type TaskStatus = "working" | "needs you" | "done" | "held";

export type BoardColumn = "backlog" | "todo" | "in-progress" | "review" | "shipping" | "done";

export interface TaskSummary {
  id: string;
  title: string;
  repo: string;
  status: TaskStatus;
  currentAgent: string;
  stage: string; // the agent the coordinator last handed it to
  column: BoardColumn; // its column on the board
  turns: number;
  needsYou: string;
  hasQuestion: boolean; // the coordinator asked you something: what you send next is the answer
  labels: string[];
  budget: number; // tokens; 0 is no limit
  tokensUsed: number;
  overBudget: boolean;
  maxTurns: number; // turns it may have without your answer; 0 is no limit
  turnsSinceAnswer: number; // since it started or you last answered
  atTurnLimit: boolean;
  stoppedAt: "" | "budget" | "turns"; // the limit whose stop is the open question: raising it answers
  rank: number; // your priority order, lower first
  parked: "" | "todo" | "backlog"; // while you hold it there
  parent: string; // the task, of another team, that asked for this one
  waitingOn: string; // another team's task this one waits for
  kind: "" | "epic"; // "" is a story
  epic: string; // a story's epic
  after: string[]; // the stories it builds on
  blockedBy: string[]; // of those, the ones not done yet, so it hasn't started
  stories: number; // an epic's
  storiesDone: number;
  closing: string; // an epic with every story done: the summary you can close it with, "" otherwise
  createdAt: string;
  lastActiveAt: string;
}

export interface TaskDetail extends TaskSummary {
  description: string;
  firstTitle: string; // the title it was created with, when the coordinator renamed it
  options: string[]; // answers the open question offers
  page: string; // the page the open question came with
  coordinator: string;
  teamChosen: boolean; // false: its coordinator may still pass it to the right team
  epicTitle: string; // a story's epic's
  storyList: TaskSummary[]; // an epic's stories, in the order they were planned
  shared: SharedFile[]; // its shared files
  movingTo: string; // the repo it moves to when the turn in progress ends
  summary: string;
  worktree: Worktree | null;
  trail: Step[] | null;
  turnList: Turn[] | null;
  approvals: Action[] | null;
  actions: Action[] | null;
  events: LogEvent[] | null;
}

export interface Worktree {
  worker: string; // the machine it is on: "local", or a worker's name
  path: string; // on that machine
  branch: string;
  base: string;
  port: number;
}

export interface Step {
  at: string;
  from: string;
  to: string;
  text: string;
  turn: string; // the turn it led to (or, said to you, came from); "" when none
}

export interface Turn {
  id: string;
  agent: string;
  worker: string; // the machine it ran on
  startedAt: string;
  lastEventAt: string; // when it ended, once it has
  acknowledged: boolean;
  ended: boolean;
  issue: string;
  tokens: number;
  plan: string; // a Markdown checklist, "" until the agent writes one
  planFrom: string; // the turn whose unfinished plan it started on, "" for a fresh one
  stopped: boolean; // you ended it, to tell the coordinator something
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
  finished: boolean; // the tool ran and returned
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
  team: string; // the team it works in; "" for Software, the team agents had before there were teams
  workers: string[]; // the machines it may run on, first online wins; none: the machine Agentry runs on ("local")
  color: string; // one of the agent colours; "" for the next free one
  column: "" | "review" | "ship"; // what its work counts as: building, review (a task is done only once they approve) or shipping
  knowledge: string; // what it has learned on earlier tasks; changed apart from its settings
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
  budget: number; // what a new task gets, in tokens; 0 is no limit
  maxTurns: number; // what a new task gets; 0 is no limit
  hasTerminal: boolean;
  hasDesk: boolean;
  engines: string[];
  workers: string[]; // the machines agents can be given: "local" first, then every worker
  canAddWorkers: boolean;
}

// A machine, besides the one Agentry runs on, that agents can run on.
export interface Worker {
  name: string;
  host: string;
  os: string;
  arch: string;
  joinedAt: string;
  online: boolean;
  since: string; // when it last came online or went offline
  version: string; // of agentry on it
  engines: string[]; // as "claude 2.1.301"
  missing: string[]; // tools agents need that it lacks
  tasks: string[]; // unfinished tasks whose work is on it
}

export interface WorkerList {
  workers: Worker[];
  local: { tasks: string[]; machine: MachineFacts };
}

// What a machine says about itself when it connects (or, for this one, when Agentry starts).
export interface MachineFacts {
  host: string;
  os: string;
  arch: string;
  version: string;
  engines: string[];
  missing: string[];
}

// A machine in detail, read from it when asked (internal/machines/about.go).
export interface MachineAbout {
  host: string;
  os: string; // "macOS 15.3", "Ubuntu 24.04.1 LTS"
  kernel: string;
  arch: string;
  cpu: string;
  cpus: number;
  memory: number; // bytes
  memoryFree: number;
  load: number[] | null; // 1, 5, 15 minutes
  bootedAt: string;
  disk: number; // where the worktrees are
  diskFree: number;
  time: string;
  zone: string;
  tools: { name: string; version: string; path: string }[];
  agentry: string;
  binary: string;
  folder: string;
  path: string[];
  running: string[];
  worktrees: string[];
  repos: string[];
}

export interface WorkerDetail {
  worker: Worker;
  about: MachineAbout | null;
  aboutError: string;
  agents: string[]; // that may run on it
  history: { at: string; what: "joined" | "online" | "offline" }[]; // newest first
}

// An install command to run on another machine, for one of the server's
// addresses (internal/web/addresses.go).
export interface InstallAddress {
  url: string;
  label: string; // such as On your network, or This computer only
  command: string; // what to run on the other machine: macOS and Linux
  windowsCommand: string; // the same, for Windows (PowerShell)
}

// The install commands for every address, and the one picked first.
export interface InstallCommands {
  command: string; // for the picked address
  windowsCommand: string;
  address: string; // the picked address's url
  addresses: InstallAddress[];
  expires: string;
}

export interface Invite extends InstallCommands {
  code: string;
}

export interface CreatedEvent {
  stream: string;
}

// Something waiting for you, from any task (internal/web/questions_api.go).
export interface Question {
  id: string;
  kind: "approval" | "question" | "budget" | "turns" | "waiting";
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
  turnsSinceAnswer?: number;
  maxTurns?: number;
}

// A one-time code that signs in another device, with the server's addresses
// for its QR code (internal/web/signin.go).
export interface SigninCode {
  code: string; // XXXX-XXXX
  expiresAt: string;
  expiresIn: number; // seconds, so a browser clock that's off doesn't matter
  addresses: { url: string; label: string }[];
  address: string; // the address this page came from, the one to show first
}

// A feature a Visionaries session suggested (internal/web/views.go ideaView).
export interface Idea {
  id: string;
  title: string;
  problem: string;
  proposal: string;
  whyNow: string;
  size: "S" | "M" | "L";
  risks: string;
  backedBy: string[];
  doubtedBy: string[];
  task: string; // the session it came from
  repo: string;
  epic: string; // the epic it follows up, "" for none
  kind: "" | "epic"; // what it suggests making of it: "" is a story
  state: "open" | "accepted" | "dismissed";
  note: string; // what you said when you accepted or dismissed it
  became: string; // the task an accepted idea became
  createdAt: string;
}

// What accepting an idea did: a task can be made and the idea still not be
// marked accepted, and then refused says why.
export interface IdeaAcceptance {
  task: string;
  accepted: boolean;
  refused: string;
}
