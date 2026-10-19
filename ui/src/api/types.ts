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
  canShip: boolean; // its team has an agent that ships work, so it may be moved to Shipping
  pushed: boolean; // an agent pushed its work since it was last reopened: a done one says Shipped
  doneByYou: boolean; // you moved it to Done, rather than its coordinator finishing it
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
  madeFrom: string; // the stream that made it, such as the idea you accepted; "" for none
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
  tokensUnknown: string; // on how many turns the tokens couldn't be counted, and why; "" when every turn was
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
  tokensUnknown?: string; // why tokens is 0, when the engine couldn't say
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
  held: boolean; // no answer came in time: the agent was told no, and it waits for you still
  heldAt?: string;
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
  effective?: EffectiveProfile; // the profile in force; none while the policy isn't loaded
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
  version: string; // this Agentry's, as the defaults it ships are
  defaults: DefaultProfile[];
  users: Record<string, ProfileUser[]>; // by each agent's own profile
  effective: Record<string, EffectiveProfile>; // by agent
  missing: Record<string, string[]>; // profiles agents have that the file lacks, with those agents
}

// A profile Agentry ships, as your file has it: missing, the default of
// this version, the default of an older one you never touched, or changed by you.
export interface DefaultProfile {
  name: string;
  state: "missing" | "current" | "outdated" | "edited";
  shippedIn?: string; // the version an outdated profile is the default of
  text: string; // as this version ships it
  diff?: string; // yours against it, when outdated or edited
}

// An agent with a profile. One that follows default_profile names none of its own.
export interface ProfileUser {
  agent: string;
  followsDefault: boolean;
}

// The profile that judges an agent's calls: its own, unless use_for_everyone overrides it.
export interface EffectiveProfile {
  profile: string;
  own: string;
  overridden: boolean;
  missing: boolean; // the file has no profile own, so the agent's calls are denied unless overridden
  text: string; // "read-mostly, overridden by allow-everything", "read-mostly (missing from policy.toml)"
}

// One entry of the policy's history, newest first.
export interface PolicyVersion {
  position: number;
  at: string;
  actor: { kind: string; id: string };
  change: string; // what changed; for a refused file, why it was refused
  refused?: boolean;
  onDisk?: boolean; // a hand edit of the file, or a file refused: Agentry only read it
  hash: string;
  diff?: string;
  backup?: string;
  canRevert: boolean; // the log holds its text
}

// What reverting to a version would change: the diff from the text in
// force to its text ("" for the version in force), and the profile each
// puts every agent under ("" when use_for_everyone isn't set).
export interface PolicyVersionChange {
  position: number;
  at: string;
  diff: string;
  useForEveryone: string;
  inForceUseForEveryone: string;
}

// A change made to the policy from the page.
export interface PolicyChange {
  hash: string;
  change: string;
  diff: string;
  backup: string;
}

// What a profile decides about a call, and the rule that decided it:
// "otherwise" when none matched.
export interface Verdict {
  decision: Decision;
  profile: string;
  rule: string;
  why: string;
  policy: string; // the hash of the version that decided
  order?: number; // the deciding rule's place in its profile, from 1; absent for built-in verdicts
}

// A call tried against a profile, run in workspace. override is what
// decides today, when use_for_everyone names another profile.
export interface TriedCall {
  verdict: Verdict;
  workspace: string;
  override?: { profile: string; verdict: Verdict };
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
  policyProblem: boolean; // policy.toml has a mistake; the Policy page says what
  missingProfiles: Record<string, string[]>; // profiles agents have that policy.toml lacks, with those agents
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
  held?: boolean; // approval: no answer came in time, so the agent was told no
  heldAt?: string;
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

// Your settings for how Agentry works (internal/web/settings_api.go).
export interface Settings {
  visionariesOnEpicClose: boolean; // a Visionaries session starts after you close an epic
  autoAcceptIdeas: boolean; // Agentry accepts an idea the Visionaries agreed on, by autoAcceptRule, as you would
  autoStartIdeas: boolean; // and starts the task it made; only with autoAcceptIdeas
  autoAcceptRule: AutoAcceptRule; // which ideas the Visionaries agreed on
}

// everyoneBacked: every viewpoint backed the idea and none doubted it.
// nobodyDoubted: none doubted it and at least two viewpoints backed it.
export type AutoAcceptRule = "everyoneBacked" | "nobodyDoubted";

// Whether the page shows one feature, and why (internal/features Line), in
// GET /api/features.
export interface FeatureLine {
  name: string; // the feature's name in the page's registry (features/index.ts)
  enabled: boolean; // its trigger is present, so it can make new data
  visible: boolean; // the page shows it: enabled, or it has data to look at
  why?: string; // one line saying why it is on, kept or off
  badge?: number; // a count for the sidebar, such as open ideas
}
