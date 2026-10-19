import { describe, expect, test } from "vitest";
import { boardColumns } from ".";

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
