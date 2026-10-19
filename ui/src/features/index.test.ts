import { describe, expect, test } from "vitest";
import type { BoardColumn, TaskSummary } from "@/api/types";
import { boardColumns, featuresOf, hiddenFeatures, unknownLine, visibleFeatures, withTheirCards } from ".";

const ALL = ["backlog", "todo", "in-progress", "review", "shipping", "done"];
const NO_SHIPPING = ["backlog", "todo", "in-progress", "review", "done"];

describe("boardColumns", () => {
  test("are the manifest's, in its order", () => {
    expect(boardColumns({ features: [], board: { columns: [...NO_SHIPPING] as never } })).toEqual(NO_SHIPPING);
    expect(boardColumns({ features: [], board: { columns: [...ALL] as never } })).toEqual(ALL);
  });

  test("leave out Shipping while the manifest loads, as any feature waits", () => {
    expect(boardColumns(undefined)).toEqual(NO_SHIPPING);
  });

  test("are every column from a server too old to list them", () => {
    expect(boardColumns({ features: [] })).toEqual(ALL);
  });

  test("skip a column this page doesn't know", () => {
    expect(boardColumns({ features: [], board: { columns: ["todo", "archive", "done"] as never } })).toEqual(["todo", "done"]);
  });
});

describe("withTheirCards", () => {
  const inColumn = (column: BoardColumn) => ({ column }) as TaskSummary;

  test("add Shipping while the manifest loads if a task is in it", () => {
    expect(withTheirCards(boardColumns(undefined), [inColumn("todo"), inColumn("shipping")])).toEqual(ALL);
  });

  test("add Shipping a loaded manifest left out if a task is in it", () => {
    const listed = boardColumns({ features: [], board: { columns: [...NO_SHIPPING] as never } });
    expect(withTheirCards(listed, [inColumn("shipping")])).toEqual(ALL);
  });

  test("change nothing when no task is in a missing column", () => {
    expect(withTheirCards(boardColumns(undefined), [inColumn("todo"), inColumn("done")])).toEqual(NO_SHIPPING);
    expect(withTheirCards([...NO_SHIPPING] as BoardColumn[], [])).toEqual(NO_SHIPPING);
  });
});

describe("hiddenFeatures", () => {
  test("is the features the manifest lists but doesn't show", () => {
    const off = featuresOf({ features: [{ name: "ideas", enabled: false, visible: false }], unknown: null });
    expect(hiddenFeatures(off).map((feature) => feature.name)).toEqual(["ideas"]);
    expect(visibleFeatures(off)).toEqual([]);
  });

  test("leaves out a visible feature, and one the manifest doesn't list", () => {
    expect(hiddenFeatures(featuresOf({ features: [{ name: "ideas", enabled: false, visible: true }], unknown: [] }))).toEqual([]);
    expect(hiddenFeatures(featuresOf({ features: [], unknown: [] }))).toEqual([]);
    expect(hiddenFeatures(featuresOf(undefined))).toEqual([]);
  });
});

describe("unknownLine", () => {
  test("words a prefix and a type", () => {
    expect(unknownLine({ prefix: "idea-", streams: 12, events: 31, last: 48211 })).toBe(
      "31 events from a feature this build doesn't have: idea-",
    );
    expect(unknownLine({ type: "git.pr_opened", events: 4, last: 48190 })).toBe(
      "4 events of a type this build doesn't know: git.pr_opened",
    );
  });

  test("counts one event as one", () => {
    expect(unknownLine({ type: "git.pr_opened", events: 1, last: 7 })).toBe("1 event of a type this build doesn't know: git.pr_opened");
  });
});
