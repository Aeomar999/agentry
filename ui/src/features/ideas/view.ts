import type { AskVisionaries } from "./visionaries";

// IdeasView is what the Ideas page shows besides the ideas and the banners:
// "waiting" until it can tell whether Ideas is on, so that nothing turns
// into another, then "on", the settings and asking the visionaries, "off",
// what Ideas is and the button that adds the team, or "unavailable", what
// Ideas is and that no team template turns it on. "unknown" is the teams
// failing to load with no ideas: only the banner saying so.
export type IdeasView = "waiting" | "on" | "off" | "unavailable" | "unknown";

export interface IdeasLoad {
  loaded: boolean; // the ideas are read
  failed: boolean; // the ideas couldn't be read
  count: number;
}

// ideasView decides it. Ideas is off only with no ideas, so a slow or failed
// load never shows the explainer to someone who has ideas, and having ideas
// is Ideas on even when the teams can't be read.
export function ideasView(ideas: IdeasLoad, teams: Pick<AskVisionaries, "read" | "ready" | "has" | "loadError">): IdeasView {
  if (ideas.failed) return "on";
  if (!ideas.loaded) return "waiting";
  if (teams.read) {
    if (ideas.count > 0 || teams.has) return "on";
    return teams.ready ? "off" : "unavailable";
  }
  if (teams.loadError) return ideas.count > 0 ? "on" : "unknown";
  return "waiting";
}

// canAsk says whether the page offers the button that asks the visionaries:
// not once the teams are read and no template turns Ideas on, since it
// could never work. While they load or can't be read it shows, disabled.
export function canAsk(teams: Pick<AskVisionaries, "read" | "ready">): boolean {
  return !teams.read || teams.ready;
}
