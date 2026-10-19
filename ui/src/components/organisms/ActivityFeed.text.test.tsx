import { describe, expect, test, vi } from "vitest";
import { render } from "@testing-library/react";
import type { Action } from "@/api/types";
import { ActivityFeed } from "./ActivityFeed";
import { AGENTS, ITEMS } from "./ActivityFeed.fixtures";

// EntryText and CodeBox stand in for themselves here, so the test reads
// what the feed asks of them: how many lines each text folds at, and as what.
const entries: { text: string; lines: number; kind: string }[] = [];
vi.mock("./EntryText", () => ({
  EntryText: (props: { text: string; lines: number; kind: string }) => {
    entries.push(props);
    return <div>{props.text}</div>;
  },
}));
const boxes: { code: string; language: string; command?: boolean }[] = [];
vi.mock("@/components/molecules/CodeBox", () => ({
  CodeBox: (props: { code: string; language: string; command?: boolean }) => {
    boxes.push(props);
    return <pre>{props.code}</pre>;
  },
}));

const read = {
  ...ITEMS[7],
  key: "20",
  body: "ui/src/lib/activity.ts",
  action: { ...ITEMS[7].action!, tool: "Read", command: "ui/src/lib/activity.ts" },
};
const items = [
  ...ITEMS,
  read,
  { ...ITEMS[0], key: "21", kind: "waiting" as const, by: "Agentry", turn: "turn-3", body: "Which one?" },
  { ...ITEMS[0], key: "22", kind: "issue" as const, by: "Agentry", turn: "turn-3", body: "The worker went away." },
  { ...ITEMS[0], key: "23", kind: "message" as const, by: "reviewer", from: "reviewer", to: "you", turn: "turn-3", body: "Done?" },
];

function feed() {
  entries.length = 0;
  boxes.length = 0;
  render(
    <ActivityFeed items={items} agents={AGENTS} links={{}} turn="" show="all" row="" now={null} onTurn={() => {}} onShow={() => {}} />,
  );
}

const entry = (text: string) => entries.find((e) => e.text === text);

describe("ActivityFeed's text", () => {
  test("goes through EntryText, folded at 10 lines for briefs, reports, questions, issues and your messages", () => {
    feed();
    expect(entry("Build the bands.")).toMatchObject({ lines: 10, kind: "brief" });
    expect(entry("## Done\n\nAll **green**.")).toMatchObject({ lines: 10, kind: "report" });
    expect(entry("Which one?")).toMatchObject({ lines: 10, kind: "question" });
    expect(entry("The worker went away.")).toMatchObject({ lines: 10, kind: "issue" });
    expect(entry("Make **Activity** readable.")).toMatchObject({ lines: 10, kind: "message" });
    expect(entry("Done?")).toMatchObject({ lines: 10, kind: "message" });
  });

  test("folds a note's text at 4 lines", () => {
    feed();
    expect(entry("one\ntwo")).toMatchObject({ lines: 4, kind: "note" });
  });

  test("puts a Bash command in a command box, which folds at 3 lines, coloured as bash", () => {
    feed();
    expect(boxes).toContainEqual({ code: "npm test", language: "bash", command: true });
    expect(boxes).toContainEqual({ code: "go test ./...", language: "bash", command: true });
  });

  test("shows another tool's input as a line of mono, not a code box", () => {
    feed();
    expect(boxes.map((box) => box.code)).not.toContain((read.action as Action).command);
    expect(entries.map((e) => e.text)).not.toContain(read.body);
  });
});
