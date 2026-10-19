import { describe, expect, test } from "vitest";
import { render } from "@testing-library/react";
import { Markdown, MarkdownProps } from "./Markdown";
import { linkInto } from "@/lib/links";
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
