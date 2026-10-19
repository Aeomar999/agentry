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

export const SCALES: Record<LimitKind, LimitScale> = {
  budget: {
    name: "Budget",
    presets: [500_000, 1_000_000, 2_000_000, 5_000_000, 10_000_000],
    show: tokens,
    parse: (text) => positive(parseTokens(text)),
    example: "A number of tokens, such as 2M or 500k.",
  },
  turns: {
    name: "Max turns",
    presets: [12, 24, 50, 100],
    show: String,
    parse: (text) => positive(/^\s*\d+\s*$/.test(text) ? Number(text) : NaN),
    example: "A whole number of turns, such as 30.",
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

export function choiceOf(scale: LimitScale, limit: number): LimitChoice {
  if (limit <= 0) return { option: UNLIMITED, custom: "" };
  if (scale.presets.includes(limit)) return { option: String(limit), custom: "" };
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

function positive(n: number): number {
  return Number.isInteger(n) && n > 0 ? n : NaN;
}
