import { parseTokens, tokens } from "./tokens";

// A task's two limits, its token budget and its turns without your answer,
// as the page picks them: a preset, Unlimited, or a number of your own. In
// the task's JSON 0 is no limit; the server takes -1 for it, since a new
// task with 0 gets the default.

export type LimitKind = "budget" | "turns";

export interface LimitScale {
  name: string; // "Budget", "Max turns"
  presets: number[];
  show: (n: number) => string; // a limit as people read it: 2M, 24
  parse: (text: string) => number; // what you typed as a limit, or NaN
  example: string; // for the error under a custom value
}

// The most a custom limit may be: far past any real task, and well inside
// what the server reads as a number.
const MAX_BUDGET = 1_000_000_000_000;
const MAX_TURNS = 100_000;

export const SCALES: Record<LimitKind, LimitScale> = {
  budget: {
    name: "Budget",
    presets: [500_000, 1_000_000, 2_000_000, 5_000_000, 10_000_000],
    show: tokens,
    parse: (text) => upTo(MAX_BUDGET, parseTokens(text)),
    example: "A number of tokens up to 1,000,000M, such as 2M or 500k.",
  },
  turns: {
    name: "Max turns",
    presets: [12, 24, 50, 100],
    show: String,
    parse: (text) => upTo(MAX_TURNS, /^\s*\d+\s*$/.test(text) ? Number(text) : NaN),
    example: "A whole number of turns up to 100,000, such as 30.",
  },
};

export const UNLIMITED = "0";
export const CUSTOM = "custom";

// What the select shows: a preset's value, UNLIMITED or CUSTOM, and the
// custom text.
export interface LimitChoice {
  option: string;
  custom: string;
}

// The limits the select lists: the presets, and a default that isn't one
// in its place among them, so it can be marked and picked like a preset.
export function optionsOf(scale: LimitScale, defaultLimit?: number): number[] {
  if (!defaultLimit || defaultLimit < 0 || scale.presets.includes(defaultLimit)) return scale.presets;
  return [...scale.presets, defaultLimit].sort((a, b) => a - b);
}

export function choiceOf(scale: LimitScale, limit: number, defaultLimit?: number): LimitChoice {
  if (limit <= 0) return { option: UNLIMITED, custom: "" };
  if (optionsOf(scale, defaultLimit).includes(limit)) return { option: String(limit), custom: "" };
  // tokens() rounds, so 1,250,000 shows as 1.3M: write it out when it would.
  const shown = scale.parse(scale.show(limit)) === limit ? scale.show(limit) : String(limit);
  return { option: CUSTOM, custom: shown };
}

// The limit chosen, 0 for none, or NaN for a custom value that isn't one.
export function limitOf(scale: LimitScale, choice: LimitChoice): number {
  return choice.option === CUSTOM ? scale.parse(choice.custom) : Number(choice.option);
}

// The number the server takes: -1 for no limit.
export function sent(limit: number): number {
  return limit === 0 ? -1 : limit;
}

// "5M" or "unlimited", for messages.
export function described(scale: LimitScale, limit: number): string {
  return limit > 0 ? scale.show(limit) : "unlimited";
}

// The most tasks Agentry gives a turn at once, across every repo: a board
// setting, not a task's, but picked the same way. 0 is no limit, on the wire
// too. The most matches the server's (settings.MostRunningTasks).
const MOST_RUNNING = 100;

export const RUNNING: LimitScale = {
  name: "Most running at once",
  presets: [1, 2, 3, 4, 5, 8, 10],
  show: String,
  parse: (text) => upTo(MOST_RUNNING, /^\s*\d+\s*$/.test(text) ? Number(text) : NaN),
  example: "A whole number of tasks from 1 to 100.",
};

// "2 of 2 running", and how many wait for a slot first: "Queued: 3 · 2 of
// 2 running". "" with no limit, when nothing waits.
export function runningLine(limit: number, running: number, queued: number): string {
  if (limit <= 0) return "";
  return (queued > 0 ? `Queued: ${queued} · ` : "") + `${running} of ${limit} running`;
}

// n when it is a whole number from 1 to max, or NaN.
function upTo(max: number, n: number): number {
  return Number.isInteger(n) && n > 0 && n <= max ? n : NaN;
}
