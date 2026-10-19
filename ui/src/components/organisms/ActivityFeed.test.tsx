import { describe, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import type { Show } from "@/lib/activity";
import { layOut } from "@/test-layout";
import { ActivityFeed, Now } from "./ActivityFeed";
import { AGENTS, ITEMS } from "./ActivityFeed.fixtures";

layOut();

// MonacoColorize stands in for Monaco: loading it is what the lazy test watches.
const loaded = vi.fn();
vi.mock("@/components/molecules/MonacoColorize", () => {
  loaded();
  return { default: ({ code }: { code: string }) => <>{code}</> };
});

interface Options {
  turn?: string;
  show?: Show;
  now?: Now | null;
  items?: typeof ITEMS;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
  foot?: ReactNode;
}

function feed({ turn = "", show = "all", now = null, items = ITEMS, ...rest }: Options = {}) {
  const onTurn = vi.fn();
  const view = render(
    <ActivityFeed
      items={items}
      agents={AGENTS}
      links={{}}
      turn={turn}
      show={show}
      row=""
      now={now}
      onTurn={onTurn}
      onShow={() => {}}
      {...rest}
    />,
  );
  return { ...view, onTurn };
}

const bands = () => screen.getAllByRole("heading", { level: 3 }).map(accessibleName);

describe("ActivityFeed", () => {
  test("starts with a heading for the tab, for screen readers", () => {
    feed();
    const heading = screen.getByRole("heading", { level: 2, name: "Activity" });
    expect(heading.className).toContain("sr-only");
  });

  test("names every band's author as copy.md says, newest first", () => {
    feed();
    expect(bands()).toEqual([
      "reviewer, Software team, turn-3, continued, running. Show only this turn",
      "you",
      "reviewer, Software team, turn-3, running. Show only this turn",
      "product-lead, Design team, coordinator",
      "engineer, Software team, turn-2, on studio, ended. Show only this turn",
      "Agentry",
      "coordinator, Software team, turn-1, ended. Show only this turn",
      "you",
    ]);
  });

  test("is one list of runs, one item each, with its band and its rows; no regions", () => {
    const { container } = feed();
    const runs = container.querySelectorAll(":scope ol > li:has(> div > h3)");
    expect(runs).toHaveLength(8);
    expect(container.querySelector("section, [role=region]")).toBeNull();
    const engineer = runs[4];
    expect(within(engineer as HTMLElement).getAllByRole("listitem")).toHaveLength(3);
    expect(engineer.querySelector("h3")!.closest("div")!.className).toContain("sticky");
  });

  test("keeps the coordinator's handoff under its turn's band, from → to with ' to ' said", () => {
    feed();
    const coordinator = screen.getByRole("button", { name: /^coordinator, Software team, turn-1/ }).closest("li")!;
    const [handoff] = within(coordinator).getAllByRole("listitem"); // the newest row, right under the band
    expect(handoff.textContent).toContain("Build the bands.");
    expect(handoff.textContent).toMatch(/coordinator to .?engineer on studio/); // .? is the chip's letter, hidden
    expect(within(coordinator).getByRole("button", { name: "Show only turn-2" })).toBeTruthy();
  });

  test("narrows to the turn from its band, and from its continued band", () => {
    const { onTurn } = feed();
    const [continued, band] = screen.getAllByRole("button", { name: /Show only this turn$/ });
    fireEvent.click(continued);
    fireEvent.click(band);
    expect(onTurn.mock.calls).toEqual([["turn-3"], ["turn-3"]]);
  });

  test("narrowed, the band is the turn's heading alone, with no Show only", () => {
    feed({ turn: "turn-3" });
    expect(bands()).toEqual(["reviewer, Software team, turn-3, running"]);
    expect(screen.getByRole("button", { name: "reviewer, Software team, turn-3, running" }).hasAttribute("disabled")).toBe(true);
  });

  test("puts the turn's facts in the band: badge, tokens, and the duration for a phone", () => {
    feed();
    const engineer = screen.getByRole("button", { name: /^engineer, Software team, turn-2/ });
    expect(engineer.textContent).toContain("turn-2");
    expect(engineer.textContent).toContain("on studio");
    expect(engineer.textContent).toContain("ended");
    expect(engineer.textContent).toContain("41k tokens");
    expect(engineer.querySelector(".md\\:hidden")!.textContent).toBe("5m 00s");
  });

  test("shows the live duration in success-ink with …, in both the turn's bands", () => {
    feed();
    const [continued, band] = screen.getAllByRole("button", { name: /^reviewer/ });
    for (const button of [continued, band]) expect(button.querySelector(".text-success-ink")!.textContent).toMatch(/…$/);
  });

  test("renders an entry's Markdown, with no raw syntax", () => {
    const { container } = feed();
    expect(screen.getByRole("heading", { level: 2, name: "Done" })).toBeTruthy();
    expect(container.textContent).not.toMatch(/\*\*|##/);
  });

  test("shows an entry with no text as its line alone, with no empty box", () => {
    const { container } = feed();
    const finished = [...container.querySelectorAll("li[data-row]")].find(
      (li) => li.textContent?.startsWith("Finished") && !li.textContent.includes("Done"),
    );
    expect(finished!.querySelectorAll("p")).toHaveLength(1);
    expect(finished!.querySelector("button")).toBeNull();
  });

  test("has no left accent edge, shadow or gradient anywhere", () => {
    const { container } = feed();
    // rounded-lg is the theme's 4px; only the rail's dots are round.
    expect(container.innerHTML).not.toMatch(/border-l-|shadow|gradient|rounded-(xl|2xl|3xl)/);
  });

  test("hides chips from screen readers", () => {
    const { container } = feed();
    for (const chip of container.querySelectorAll("h3 [class*='size-5']")) expect(chip.getAttribute("aria-hidden")).toBe("true");
  });

  test("keeps screen-reader text in the tab's own scroller, its positioned box, so the page doesn't grow", () => {
    const { container } = feed();
    const said = [...container.querySelectorAll(".sr-only")].find((span) => span.textContent === " to ")!;
    expect(said.parentElement!.closest(".relative")!.className).toContain("overflow-y-auto");
  });

  test("sticks bands above everything in the rows (z-10), under the top bar (z-20)", () => {
    const { container } = feed();
    for (const band of container.querySelectorAll("h3")) expect(band.parentElement!.className).toContain("sticky top-0 z-10");
  });

  test("shows the rail time as hh:mm at every width, on one line", () => {
    const { container } = feed();
    const times = [...container.querySelectorAll(".whitespace-nowrap > span")].filter((span) => span.textContent!.includes(":"));
    expect(times.length).toBeGreaterThan(0);
    for (const time of times) {
      expect(time.textContent).toMatch(/^\d\d:\d\d$/);
      expect(time.className).not.toMatch(/hidden/);
    }
  });

  test("loads no Monaco while no command is near the screen", async () => {
    feed();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(screen.getByText("go test ./...")).toBeTruthy();
    expect(loaded).not.toHaveBeenCalled();
  });

  describe("states", () => {
    test("a new task", () => {
      feed({ items: [] });
      expect(screen.getByText("Nothing here yet")).toBeTruthy();
      expect(screen.getByText("Every message, turn and action shows here as it happens.")).toBeTruthy();
    });

    test.each([
      ["actions", "No actions yet"],
      ["messages", "No messages yet"],
    ] as const)("a %s filter with nothing to show", (show, title) => {
      feed({ items: [ITEMS[4]], show });
      expect(screen.getByText(title)).toBeTruthy();
      expect(screen.getByText("Choose Everything to see every row.")).toBeTruthy();
    });

    test("loading: still blocks, the words for screen readers, and busy", () => {
      const { container } = feed({ loading: true });
      expect(screen.getByText("Loading activity…").className).toContain("sr-only");
      expect(container.querySelector("[aria-busy=true]")).toBeTruthy();
      expect(container.innerHTML).not.toMatch(/animate-pulse/);
      expect(screen.getByText("Everything")).toBeTruthy(); // the toolbar stays
    });

    test("an error, with what the server said and Try again", () => {
      const retry = vi.fn();
      feed({ error: "GET /api/tasks/task-40: 502 Bad Gateway", onRetry: retry });
      const alert = screen.getByRole("alert");
      expect(alert.textContent).toContain("Couldn't load the activity");
      expect(alert.textContent).toContain("GET /api/tasks/task-40: 502 Bad Gateway");
      expect(alert.textContent).toContain("If it keeps failing, check that Agentry is still running.");
      fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));
      expect(retry).toHaveBeenCalledOnce();
    });

    test("a turn waiting for the usage limit: its band says so, and the row says until when", () => {
      const band = ITEMS[10];
      const items = [
        ...ITEMS.slice(0, 10),
        { ...band, live: false, tone: "warning" as const, turnInfo: { ...band.turnInfo!, limited: "You've hit your limit" } },
        { ...ITEMS[11], key: "12", kind: "waiting" as const, title: "Waiting for the usage limit", body: "Resets at 07:00" },
      ];
      feed({ items });
      expect(bands()[0]).toBe("reviewer, Software team, turn-3, usage limit. Show only this turn");
      expect(screen.getByText("Waiting for the usage limit")).toBeTruthy();
      expect(screen.queryByText("running")).toBeNull();
    });

    test("puts the way to read earlier events under the oldest row, in the scroller", () => {
      const { container } = feed({ foot: <button type="button">Load earlier</button> });
      const list = container.querySelector("ol")!;
      const button = screen.getByRole("button", { name: "Load earlier" });
      expect(list.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(button.parentElement).toBe(list.parentElement);
    });

    test("a turn just started, with no rows: its band, then who is starting", () => {
      feed({ items: ITEMS.slice(0, 11), now: { label: "reviewer is working", tone: "success" } });
      expect(bands()[0]).toBe("reviewer, Software team, turn-3, running. Show only this turn");
      expect(screen.getByText("reviewer is starting…").className).toContain("text-success-ink");
      expect(screen.queryByText("reviewer is working")).toBeNull();
    });
  });
});

// accessibleName is a heading's name: its own label, or its button's.
function accessibleName(heading: HTMLElement): string {
  return heading.getAttribute("aria-label") ?? heading.querySelector("button")?.getAttribute("aria-label") ?? heading.textContent!;
}
