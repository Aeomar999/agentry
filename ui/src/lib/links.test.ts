import { describe, expect, test } from "vitest";
import { safeHref } from "./links";
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
