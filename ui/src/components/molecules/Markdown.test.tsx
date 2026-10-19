import { describe, expect, test } from "vitest";
import { render } from "@testing-library/react";
import { Markdown, MarkdownProps } from "./Markdown";
import { linkInto, refTo, Refs } from "@/lib/links";
import { OTHER_SITES, UNSAFE } from "@/lib/links.fixtures";

const hook = (href: string) => linkInto("task-75", href);

// Every link test runs twice: on Markdown's own check, and through the chat's hook.
const HOSTS: [string, Partial<MarkdownProps>][] = [
  ["without a link hook", {}],
  ["with a link hook", { link: hook }],
];

function links(text: string, props: Partial<MarkdownProps> = {}) {
  const { container } = render(<Markdown text={text} {...props} />);
  return [...container.querySelectorAll("a")];
}

describe.each(HOSTS)("Markdown links, %s", (_, props) => {
  test.each(UNSAFE)("draws [x](%j) as its label, no link", (raw) => {
    const { container } = render(<Markdown text={`see [x](${raw}) here`} {...props} />);
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("x");
  });

  test("draws a bare javascript: in prose as text", () => {
    expect(links("try javascript:alert(1) now", props)).toEqual([]);
  });

  test.each(OTHER_SITES)("sends [x](%j) to https://evil.example/ in a new tab, never the raw text", (raw) => {
    const [a] = links(`[x](${raw})`, props);
    expect(a.getAttribute("href")).toBe("https://evil.example/");
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
  });

  test.each(["https://example.com", "HTTPS://EXAMPLE.COM/x", "mailto:a@example.com"])("opens [x](%j) in a new tab", (raw) => {
    const [a] = links(`[x](${raw})`, props);
    expect(a.getAttribute("href")).toBe(new URL(raw).href);
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
  });

  test("autolinks a bare https:// link", () => {
    const [a] = links("see https://example.com/x.", props);
    expect(a.getAttribute("href")).toBe("https://example.com/x");
    expect(a.getAttribute("target")).toBe("_blank");
  });

  test("draws a path above the root as its label", () => {
    expect(links("[x](../../etc/passwd)", props)).toEqual([]);
  });

  test("keeps an anchor on the page", () => {
    const [a] = links("[x](#strings)", props);
    expect(a.getAttribute("href")).toBe("#strings");
    expect(a.getAttribute("target")).toBeNull();
  });
});

describe("Markdown repo paths", () => {
  test.each(["docs/design.md", "./docs/design.md", "/docs/design.md"])("without a hook, renders [x](%j) as docs/design.md", (raw) => {
    expect(links(`[x](${raw})`)[0].getAttribute("href")).toBe("docs/design.md");
  });

  test.each(["docs/design.md", "./docs/design.md", "/docs/design.md"])("with the hook, opens [x](%j) in the Workspace", (raw) => {
    const [a] = links(`[x](${raw})`, { link: hook });
    expect(a.getAttribute("href")).toBe("/tasks/task-75/workspace?file=docs%2Fdesign.md&view=preview");
    expect(a.getAttribute("target")).toBe("_blank");
  });

  test("refuses a hook's link that safeHref wouldn't let through", () => {
    expect(links("[x](docs/a.md)", { link: () => ({ href: "javascript:alert(1)", external: false }) })).toEqual([]);
  });
});

describe("Markdown marks", () => {
  test("marks a new-tab link with ↗ and says so to screen readers", () => {
    const [a] = links("[the design](docs/design.md)", { link: hook, marks: true });
    expect(a.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
    expect(a.textContent).toBe("the design (opens in a new tab)");
    expect(a.className).toContain("text-link");
  });

  test("leaves an anchor unmarked", () => {
    const [a] = links("[x](#strings)", { marks: true });
    expect(a.querySelector("svg")).toBeNull();
    expect(a.textContent).toBe("x");
  });

  test("draws links as before without marks", () => {
    const [a] = links("[x](https://example.com)", { link: hook });
    expect(a.querySelector("svg")).toBeNull();
    expect(a.textContent).toBe("x");
    expect(a.className).toContain("text-info");
  });
});

const refs: Refs = {
  titles: new Map([["task-21", "Fix the chat links"]]),
  commits: ["3f2a9c1d0e4b5a6978877665544332211aabbccd"],
};
const withRefs: Partial<MarkdownProps> = {
  link: (href) => linkInto("task-75", href, refs),
  refs: (word) => refTo("task-75", word, refs),
  marks: true,
};

describe("Markdown refs", () => {
  test.each([
    ["bare", "see task-21."],
    ["in inline code", "see `task-21`."],
    ["linked", "see [task-21](task-21)."],
  ])("links a task the board knows, %s, in the same tab with its title", (_, text) => {
    const [a] = links(text, withRefs);
    expect(a.getAttribute("href")).toBe("/tasks/task-21");
    expect(a.getAttribute("target")).toBeNull();
    expect(a.getAttribute("title")).toBe("Fix the chat links");
    expect(a.getAttribute("aria-label")).toBe("task-21, Fix the chat links");
    expect(a.querySelector("svg")).toBeNull();
  });

  test.each(["see 3f2a9c1 there", "see `3f2a9c1` there"])("links a commit of the task in %j to its full hash", (text) => {
    const [a] = links(text, withRefs);
    expect(a.getAttribute("href")).toBe("/tasks/task-75/workspace?commit=3f2a9c1d0e4b5a6978877665544332211aabbccd");
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("aria-label")).toBe("commit 3f2a9c1 (opens in a new tab)");
    expect(a.textContent).toBe("3f2a9c1 "); // the label, then a space before ↗
    expect(a.querySelector("svg")).not.toBeNull();
  });

  test("draws linked inline code as a mono link, with no code background", () => {
    const { container } = render(<Markdown text="see `task-21`" {...withRefs} />);
    const a = container.querySelector("a")!;
    expect(container.querySelector("code")).toBeNull();
    expect(a.className).toContain("font-mono");
    expect(a.className).not.toContain("bg-surface-3");
  });

  test.each([
    ["an unknown task", "task-999"],
    ["an unknown hash", "abcdef1"],
    ["a hash too short", "3f2a9c"],
  ])("shows %s as written", (_, word) => {
    const { container } = render(<Markdown text={`bare ${word} and \`${word}\``} {...withRefs} />);
    expect(container.querySelector("a")).toBeNull();
    expect(container.querySelector("code")?.textContent).toBe(word);
    expect(container.textContent).toBe(`bare ${word} and ${word}`);
  });

  test("keeps the words around a ref", () => {
    const { container } = render(<Markdown text="before task-21 and task-21, after" {...withRefs} />);
    expect(container.textContent).toBe("before task-21 and task-21, after");
    expect(container.querySelectorAll("a")).toHaveLength(2);
  });

  test("puts no link inside a link's label", () => {
    const anchors = links("[task-21 notes](https://example.com)", withRefs);
    expect(anchors).toHaveLength(1);
    expect(anchors[0].getAttribute("href")).toBe("https://example.com/");
  });
});
