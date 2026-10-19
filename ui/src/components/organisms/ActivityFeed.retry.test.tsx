import { describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import type { LogEvent, TaskDetail } from "@/api/types";
import { activityOf } from "@/lib/activity";
import { EventPages } from "@/live/event-pages";
import { useEvents } from "@/live/use-events";
import { layOut } from "@/test-layout";
import { ActivityFeed } from "./ActivityFeed";
import { AGENTS } from "./ActivityFeed.fixtures";

layOut();

vi.mock("@/components/molecules/MonacoColorize", () => ({ default: ({ code }: { code: string }) => <>{code}</> }));

// The page's event lists, read through a fetch the test answers: every read
// waits until the test fails it or answers it with the task's one event.
const reads: { resolve: (read: { data: unknown; position: number }) => void; reject: (failure: Error) => void }[] = [];
const pages = new EventPages(() => new Promise((resolve, reject) => reads.push({ resolve, reject })));
vi.mock("@/live/LiveProvider", () => ({ useEventPages: () => pages }));

const TASK = { coordinator: "coordinator", turnList: [], actions: [] } as unknown as TaskDetail;
const CREATED: LogEvent = {
  position: 1,
  at: "2026-10-19T10:00:00Z",
  type: "task.created",
  actor: "human:you",
  data: { title: "Make it readable" },
};

// Activity is the Activity tab as TaskPage wires it.
function Activity() {
  const log = useEvents({ task: "task-40", newestBefore: 2 });
  return (
    <ActivityFeed
      items={activityOf(TASK, log.events)}
      agents={AGENTS}
      links={{}}
      turn=""
      show="all"
      row=""
      now={null}
      loading={log.loading && !log.error}
      error={log.events.length ? undefined : log.error}
      onRetry={log.retry}
      onTurn={() => {}}
      onShow={() => {}}
    />
  );
}

const fail = (message: string) => act(async () => reads.shift()!.reject(new Error(message)));
const answer = () => act(async () => reads.shift()!.resolve({ data: { events: [CREATED], more: false }, position: 1 }));
const tryAgain = () => fireEvent.click(screen.getByRole("button", { name: "Try again" }));
const feedBox = () => screen.getByRole("region", { name: "Activity" }); // the scroller

describe("Activity: Try again after the events couldn't be read", () => {
  test("a failed read, then Try again, then the feed, with focus on it and no other read", async () => {
    const { container, unmount } = render(<Activity />);
    await fail("GET /api/tasks/task-40/events: 502 Bad Gateway");
    expect(screen.getByRole("alert").textContent).toContain("Couldn't load the activity");

    tryAgain();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByText("Loading activity…")).toBeTruthy();
    expect(container.querySelector("[aria-busy=true]")).toBeTruthy();
    expect(document.activeElement).toBe(feedBox()); // not dropped on <body> with the button
    expect(reads).toHaveLength(1);

    await answer();
    expect(screen.getByText("Make it readable")).toBeTruthy();
    expect(container.querySelector("[aria-busy=true]")).toBeNull();
    expect(document.activeElement).toBe(feedBox());
    unmount();
  });

  test("a retry that fails again shows the alert again; two quick clicks read once", async () => {
    const { unmount } = render(<Activity />);
    await fail("502 Bad Gateway");
    const button = screen.getByRole("button", { name: "Try again" });
    fireEvent.click(button);
    fireEvent.click(button); // the second, on the button the first took away
    expect(reads).toHaveLength(1);

    await fail("503 Service Unavailable");
    expect(screen.getByRole("alert").textContent).toContain("503 Service Unavailable");
    // Focus waited on the feed, a Tab stop; the next Tab from there is the new alert's button.
    expect(document.activeElement).toBe(feedBox());
    expect(feedBox().tabIndex).toBe(0);
    const again = screen.getByRole("button", { name: "Try again" });
    expect(feedBox().contains(again)).toBe(true);
    expect(again.tabIndex).toBe(0);
    tryAgain();
    expect(reads).toHaveLength(1);
    await answer();
    expect(screen.getByText("Make it readable")).toBeTruthy();
    unmount();
  });
});
