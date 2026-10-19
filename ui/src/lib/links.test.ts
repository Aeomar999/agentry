import { describe, expect, test } from "vitest";
import { linkInto, refTo, Refs, safeHref } from "./links";
import { OTHER_SITES, UNSAFE } from "./links.fixtures";

describe("safeHref", () => {
  test.each(UNSAFE)("refuses %j", (raw) => {
    expect(safeHref(raw)).toBeNull();
  });

  test.each(OTHER_SITES)("reads %j as another site, rendered as https://evil.example/", (raw) => {
    expect(safeHref(raw)).toEqual({ kind: "external", href: "https://evil.example/" });
  });

  test.each([
    ["https://example.com", "https://example.com/"],
    ["HTTPS://EXAMPLE.COM/x", "https://example.com/x"],
    ["http://example.com/a?b=1#c", "http://example.com/a?b=1#c"],
    ["mailto:a@example.com", "mailto:a@example.com"],
    [" https://example.com ", "https://example.com/"],
  ])("lets %j through to the web as %j", (raw, href) => {
    expect(safeHref(raw)).toEqual({ kind: "external", href });
  });

  test("refuses a web link that doesn't parse", () => {
    expect(safeHref("https://exa mple.com")).toBeNull();
  });

  test.each(["docs/design.md", "./docs/design.md", "/docs/design.md", "docs/./design.md", "docs/x/../design.md", "docs\\design.md"])(
    "reads %j as the repo path docs/design.md",
    (raw) => {
      expect(safeHref(raw)).toEqual({ kind: "path", href: "docs/design.md" });
    },
  );

  test("keeps a path's anchor", () => {
    expect(safeHref("./docs/design.md#links")).toEqual({ kind: "path", href: "docs/design.md#links" });
  });

  test.each(["../../etc/passwd", "../x", "docs/../../x", "%2e%2e/x", "/", "./", ""])("refuses %j, above or at the root", (raw) => {
    expect(safeHref(raw)).toBeNull();
  });

  test("keeps an anchor on the page", () => {
    expect(safeHref("#strings")).toEqual({ kind: "anchor", href: "#strings" });
  });
});

const refs: Refs = {
  titles: new Map([
    ["task-21", "Fix the chat links"],
    ["task-22", ""],
  ]),
  commits: ["3f2a9c1d0e4b5a6978877665544332211aabbccd", "9e1d04bffffffffffffffffffffffffffffffff0"],
};

describe("refTo", () => {
  test("links a task the board knows, in the same tab, with its title", () => {
    expect(refTo("task-75", "task-21", refs)).toEqual({
      href: "/tasks/task-21",
      external: false,
      title: "Fix the chat links",
      name: "task-21, Fix the chat links",
    });
  });

  test("links a task with no title yet without a tooltip", () => {
    expect(refTo("task-75", "task-22", refs)).toEqual({ href: "/tasks/task-22", external: false });
  });

  test("leaves a task the board doesn't know as written", () => {
    expect(refTo("task-75", "task-999", refs)).toBeNull();
  });

  test.each(["3f2a9c1", "3F2A9C1", "3f2a9c1d0e4b", "3f2a9c1d0e4b5a6978877665544332211aabbccd"])(
    "links %j, the start of one of the task's commits, to its full hash in a new tab",
    (word) => {
      expect(refTo("task-75", word, refs)).toEqual({
        href: "/tasks/task-75/workspace?commit=3f2a9c1d0e4b5a6978877665544332211aabbccd",
        external: true,
        name: `commit ${word} (opens in a new tab)`,
      });
    },
  );

  test.each([
    ["an unknown hash", "abcdef1"],
    ["a hash too short", "3f2a9c"],
    ["a hash too long", "3f2a9c1d0e4b5a6978877665544332211aabbccd0"],
    ["a word that isn't hex", "3f2a9cz"],
  ])("leaves %s as written", (_, word) => {
    expect(refTo("task-75", word, refs)).toBeNull();
  });

  test("links nothing while the commits haven't come", () => {
    expect(refTo("task-75", "3f2a9c1", { ...refs, commits: [] })).toBeNull();
  });
});

describe("linkInto", () => {
  test.each(UNSAFE)("refuses %j", (raw) => {
    expect(linkInto("task-75", raw)).toBeNull();
  });

  test.each(OTHER_SITES)("sends %j to https://evil.example/ in a new tab", (raw) => {
    expect(linkInto("task-75", raw)).toEqual({ href: "https://evil.example/", external: true });
  });

  test("sends the web there, in a new tab", () => {
    expect(linkInto("task-75", "https://example.com/x")).toEqual({ href: "https://example.com/x", external: true });
  });

  test.each(["docs/design.md", "./docs/design.md", "/docs/design.md", "docs/design.md#links"])(
    "opens %j in the task's Workspace preview, in a new tab",
    (raw) => {
      expect(linkInto("task-75", raw)).toEqual({
        href: "/tasks/task-75/workspace?file=docs%2Fdesign.md&view=preview",
        external: true,
      });
    },
  );

  test("refuses a path above the root", () => {
    expect(linkInto("task-75", "../../etc/passwd")).toBeNull();
  });

  test("keeps an anchor on the page", () => {
    expect(linkInto("task-75", "#strings")).toEqual({ href: "#strings", external: false });
  });

  test("with refs, links a task or a commit named as the target", () => {
    expect(linkInto("task-75", "task-21", refs)?.href).toBe("/tasks/task-21");
    expect(linkInto("task-75", "3f2a9c1", refs)?.href).toBe("/tasks/task-75/workspace?commit=3f2a9c1d0e4b5a6978877665544332211aabbccd");
  });

  test("without refs, a task id is a file like any other", () => {
    expect(linkInto("task-75", "task-21")?.href).toBe("/tasks/task-75/workspace?file=task-21&view=preview");
  });
});
