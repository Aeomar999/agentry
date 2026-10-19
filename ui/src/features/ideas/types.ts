import type { BoardColumn } from "@/api/types";

// A feature a Visionaries session suggested (internal/ideas/routes.go ideaView).
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
  about: string; // the epic whose close started its session, "" for none
  aboutTitle: string; // that epic's title
  kind: "" | "epic"; // what it suggests making of it: "" is a story
  state: "open" | "accepted" | "dismissed";
  note: string; // what you said when you accepted or dismissed it
  decidedAt?: string; // when it was accepted or dismissed; absent while it is open
  became: string; // the task an accepted idea became
  becameColumn: BoardColumn | ""; // that task's column now, "" when it can't be read
  autoAccepted: boolean; // Agentry accepted it, as the Visionaries agreed by your rule, not you
  repeats: string; // an idea the Visionaries said it raises again, "" for none
  notAutoAccepted: string; // for an open idea, why Agentry wouldn't accept it for you under your rule; "" when it would
  createdAt: string;
}

// What accepting an idea did: a task can be made and the idea still not be
// marked accepted, and then refused says why.
export interface IdeaAcceptance {
  task: string;
  column: BoardColumn | ""; // the task's column now, "" when it can't be read; a retry keeps the task where it was made
  accepted: boolean;
  refused: string;
}

// The open task an idea looks like, which an accept without anyway is
// refused for (internal/ideas/routes.go duplicateView, in the 409's body).
export interface IdeaDuplicate {
  id: string;
  title: string;
  column: BoardColumn | ""; // "" when it can't be read
}
