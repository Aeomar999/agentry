import { describe, expect, test } from "vitest";
import type { LogEvent } from "@/api/types";
import { EventPages, EventsWanted, noEvents } from "./event-pages";

// A fake server holding one task's events at the positions given: it
// answers each page read as the real route does, with a page size of 3,
// when the test says, at the log position the test says.
function fakeServer(positions: number[]) {
  const limit = 3;
  const log: LogEvent[] = positions.map(eventAt);
  const reads: string[] = [];
  const pending: { path: string; answer: (position: number) => Promise<void>; fail: (message: string) => Promise<void> }[] = [];
  const fetch = (path: string) =>
    new Promise<{ data: unknown; position?: number }>((resolve, reject) => {
      reads.push(path);
      pending.push({
        path,
        answer: async (position) => {
          resolve({ data: pageOf(log, path, position, limit), position });
          await settle();
        },
        fail: async (message) => {
          reject(new Error(message));
          await settle();
        },
      });
    });
  const answer = async (position: number) => {
    for (const read of pending.splice(0)) await read.answer(position);
  };
  const add = (...more: number[]) => log.push(...more.map(eventAt));
  // restart puts the server on another log: a restored or different database.
  const restart = (...on: number[]) => log.splice(0, log.length, ...on.map(eventAt));
  return { fetch, reads, pending, answer, add, restart };
}

function eventAt(position: number): LogEvent {
  return { position, at: "2026-10-19T10:00:00Z", type: position % 2 ? "task.labeled" : "action.result", actor: "human:you" };
}

// pageOf is listEvents' page: after and before both left out, the first
// events after ?after=, or the last before ?before= when it is all given.
function pageOf(log: LogEvent[], path: string, readAt: number, limit: number) {
  const query = new URLSearchParams(path.split("?")[1]);
  const after = Number(query.get("after") ?? 0);
  const before = Number(query.get("before") ?? Infinity);
  const types = query.getAll("type");
  const between = log.filter(
    (e) => e.position > after && e.position < before && e.position <= readAt && (!types.length || types.includes(e.type)),
  );
  const fromEnd = query.has("before") && !query.has("after");
  const events = fromEnd ? between.slice(-limit) : between.slice(0, limit);
  return { events, more: between.length > limit };
}

const settle = () => new Promise((resume) => setTimeout(resume, 0));
const ignore = () => {};
const positions = (pages: EventPages, wanted: EventsWanted) => pages.snapshot(wanted).events.map((e) => e.position);

describe("a task's events, newest first (the Activity and Events tabs)", () => {
  const newest: EventsWanted = { task: "task-8", newestBefore: 11 }; // the detail was read at 10

  test("reads the newest page, before where the detail was read, and nothing more while the log stays", async () => {
    const server = fakeServer([1, 2, 3, 5, 8, 9, 10]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    expect(pages.snapshot(newest).loading).toBe(true);
    pages.moved(10);
    await server.answer(10);
    expect(server.reads).toEqual(["/api/tasks/task-8/events?before=11"]);
    expect(positions(pages, newest)).toEqual([8, 9, 10]);
    expect(pages.snapshot(newest)).toMatchObject({ loading: false, earlier: true });

    pages.moved(10); // the poll answering again at the same position
    await settle();
    expect(server.reads).toHaveLength(1);
  });

  test("load earlier reads the page before the oldest held, until there is none", async () => {
    const server = fakeServer([1, 2, 3, 5, 8, 9, 10]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    pages.moved(10);
    await server.answer(10);

    pages.loadEarlier(newest);
    pages.loadEarlier(newest); // a second click while it is read asks nothing more
    expect(pages.snapshot(newest).loadingEarlier).toBe(true);
    await server.answer(10);
    expect(server.reads.slice(1)).toEqual(["/api/tasks/task-8/events?before=8"]);
    expect(positions(pages, newest)).toEqual([2, 3, 5, 8, 9, 10]);
    expect(pages.snapshot(newest)).toMatchObject({ earlier: true, loadingEarlier: false });

    pages.loadEarlier(newest);
    await server.answer(10);
    expect(server.reads.slice(2)).toEqual(["/api/tasks/task-8/events?before=2"]);
    expect(positions(pages, newest)).toEqual([1, 2, 3, 5, 8, 9, 10]);
    expect(pages.snapshot(newest).earlier).toBe(false);

    pages.loadEarlier(newest);
    await settle();
    expect(server.reads).toHaveLength(3);
  });

  test("catches up from the newest held when the log moves past it, once per move, and not when it moves elsewhere", async () => {
    const server = fakeServer([1, 2, 3, 5, 8, 9, 10]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    pages.moved(10);
    await server.answer(10);

    server.add(12, 13);
    pages.moved(13);
    pages.moved(13);
    await server.answer(13);
    expect(server.reads.slice(1)).toEqual(["/api/tasks/task-8/events?after=10"]);
    expect(positions(pages, newest)).toEqual([8, 9, 10, 12, 13]);

    pages.moved(14); // an event of another task: read once, finds nothing
    await server.answer(14);
    pages.moved(14);
    await settle();
    expect(server.reads.slice(2)).toEqual(["/api/tasks/task-8/events?after=13"]);
    expect(positions(pages, newest)).toEqual([8, 9, 10, 12, 13]);
  });

  test("a catch-up that finds more than a page reads on until it has them all", async () => {
    const server = fakeServer([1, 2, 3]);
    const pages = new EventPages(server.fetch);
    const wanted = { task: "task-8", newestBefore: 4 };
    pages.watch(wanted, ignore);
    pages.moved(3);
    await server.answer(3);
    server.add(4, 5, 6, 7, 8);
    pages.moved(8);
    await server.answer(8);
    await server.answer(8);
    expect(server.reads.slice(1)).toEqual(["/api/tasks/task-8/events?after=3", "/api/tasks/task-8/events?after=6"]);
    expect(positions(pages, wanted)).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
  });

  test("events the log gains while the newest page is read are caught up after it, and none is lost", async () => {
    const server = fakeServer([1, 2, 3]);
    const pages = new EventPages(server.fetch);
    const wanted = { task: "task-8", newestBefore: 4 };
    pages.watch(wanted, ignore);
    server.add(4);
    pages.moved(4);
    await server.answer(4); // the page stops before 4, as asked
    expect(positions(pages, wanted)).toEqual([1, 2, 3]);
    await server.answer(4);
    expect(server.reads).toEqual(["/api/tasks/task-8/events?before=4", "/api/tasks/task-8/events?after=3"]);
    expect(positions(pages, wanted)).toEqual([1, 2, 3, 4]);
  });

  test("load earlier asked while a catch-up is read waits for it, then reads once, and the list has no gaps or repeats", async () => {
    const server = fakeServer([1, 2, 3, 5, 8, 9, 10]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    pages.moved(10);
    await server.answer(10);

    server.add(12, 13);
    pages.moved(13); // the catch-up is in flight
    pages.loadEarlier(newest);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks/task-8/events?after=10"]);
    expect(pages.snapshot(newest).loadingEarlier).toBe(true);

    await server.answer(13);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks/task-8/events?before=8"]);
    server.add(14);
    pages.moved(14); // the log moves again while the earlier page is read
    expect(server.pending).toHaveLength(1);

    await server.answer(14);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks/task-8/events?after=13"]);
    await server.answer(14);
    expect(server.reads).toEqual([
      "/api/tasks/task-8/events?before=11",
      "/api/tasks/task-8/events?after=10",
      "/api/tasks/task-8/events?before=8",
      "/api/tasks/task-8/events?after=13",
    ]);
    expect(positions(pages, newest)).toEqual([2, 3, 5, 8, 9, 10, 12, 13, 14]);
    expect(pages.snapshot(newest)).toMatchObject({ earlier: true, loadingEarlier: false });
  });

  test("the two tabs, and a view shown twice over, share one list and its reads", async () => {
    const server = fakeServer([1, 2, 3]);
    const pages = new EventPages(server.fetch);
    const stop = pages.watch(newest, ignore);
    stop(); // React's strict mode shows a view, takes it away and shows it again
    pages.watch({ ...newest, newestBefore: 12 }, ignore); // a later detail's bound is the same list
    pages.watch(newest, ignore);
    await server.answer(10);
    expect(server.reads).toEqual(["/api/tasks/task-8/events?before=11"]);
  });

  test("a failed read shows its error, keeps what is held and is tried again when the log moves", async () => {
    const server = fakeServer([1, 2, 3, 5]);
    const pages = new EventPages(server.fetch);
    const wanted = { task: "task-8", newestBefore: 6 };
    pages.watch(wanted, ignore);
    pages.moved(5);
    await server.answer(5);
    server.add(6);
    pages.moved(6);
    await server.pending.splice(0)[0].fail("database is locked");
    expect(pages.snapshot(wanted)).toMatchObject({ error: "database is locked" });
    expect(positions(pages, wanted)).toEqual([2, 3, 5]);
    await settle();
    expect(server.reads).toHaveLength(2); // not tried again in a loop

    server.add(7);
    pages.moved(7);
    await server.answer(7);
    expect(server.reads.slice(2)).toEqual(["/api/tasks/task-8/events?after=5"]);
    expect(pages.snapshot(wanted)).toMatchObject({ error: "" });
    expect(positions(pages, wanted)).toEqual([2, 3, 5, 6, 7]);
  });

  test("retry reads a failed first page again at once, loading again, and once however often it is asked", async () => {
    const server = fakeServer([1, 2, 3, 5]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    await server.pending.splice(0)[0].fail("502 Bad Gateway");
    expect(pages.snapshot(newest)).toMatchObject({ loading: true, error: "502 Bad Gateway" });

    pages.retry(newest);
    pages.retry(newest); // a second click before the read is back
    expect(pages.snapshot(newest)).toMatchObject({ loading: true, error: "" });
    expect(server.reads).toEqual(["/api/tasks/task-8/events?before=11", "/api/tasks/task-8/events?before=11"]);
    await server.answer(5);
    expect(positions(pages, newest)).toEqual([2, 3, 5]);
    expect(pages.snapshot(newest)).toMatchObject({ loading: false, error: "" });

    pages.retry(newest); // nothing failed
    expect(server.reads).toHaveLength(2);
  });

  test("a retry that fails again shows the error again, and can be retried", async () => {
    const server = fakeServer([1, 2, 3, 5]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    await server.pending.splice(0)[0].fail("502 Bad Gateway");
    pages.retry(newest);
    await server.pending.splice(0)[0].fail("503 Service Unavailable");
    expect(pages.snapshot(newest)).toMatchObject({ loading: true, error: "503 Service Unavailable" });
    pages.retry(newest);
    await server.answer(5);
    expect(server.reads).toHaveLength(3);
    expect(positions(pages, newest)).toEqual([2, 3, 5]);
  });

  test("retry reads a failed earlier page again, keeping what is held", async () => {
    const server = fakeServer([1, 2, 3, 5, 8, 9, 10]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    await server.answer(10);
    pages.loadEarlier(newest);
    await server.pending.splice(0)[0].fail("database is locked");
    expect(pages.snapshot(newest)).toMatchObject({ loadingEarlier: false, earlier: true, error: "database is locked" });

    pages.retry(newest);
    expect(pages.snapshot(newest)).toMatchObject({ loadingEarlier: true, error: "" });
    await server.answer(10);
    expect(server.reads.slice(1)).toEqual(["/api/tasks/task-8/events?before=8", "/api/tasks/task-8/events?before=8"]);
    expect(positions(pages, newest)).toEqual([2, 3, 5, 8, 9, 10]);
    expect(pages.snapshot(newest)).toMatchObject({ loadingEarlier: false, error: "" });
  });

  test("a retry of a list nobody shows any more reads nothing, and one dropped mid-read isn't shown", async () => {
    const server = fakeServer([1, 2, 3]);
    const pages = new EventPages(server.fetch);
    const stop = pages.watch(newest, ignore);
    await server.pending.splice(0)[0].fail("502 Bad Gateway");
    pages.retry(newest);
    stop(); // the tab closed while the retry was in flight
    await server.answer(10);
    expect(pages.snapshot(newest)).toEqual(noEvents);
    pages.retry(newest);
    expect(server.reads).toHaveLength(2);
  });

  test("a list nobody shows any more is not read again, and is forgotten", async () => {
    const server = fakeServer([1, 2, 3]);
    const pages = new EventPages(server.fetch);
    const stop = pages.watch(newest, ignore);
    await server.answer(10);
    stop();
    pages.moved(20);
    await settle();
    expect(server.reads).toHaveLength(1);
    expect(pages.snapshot(newest).loading).toBe(true);
  });
});

describe("a task's events of some types, all of them (the chat)", () => {
  const chat: EventsWanted = { task: "task-8", types: ["task.labeled"] };

  test("walks every page from the start, showing nothing until it has them all, then catches up", async () => {
    const server = fakeServer([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13]);
    const pages = new EventPages(server.fetch);
    pages.watch(chat, ignore);
    pages.moved(13);
    await server.answer(13);
    expect(pages.snapshot(chat).loading).toBe(true);
    await server.answer(13);
    await server.answer(13);
    expect(server.reads).toEqual([
      "/api/tasks/task-8/events?type=task.labeled&after=0",
      "/api/tasks/task-8/events?type=task.labeled&after=5",
      "/api/tasks/task-8/events?type=task.labeled&after=11",
    ]);
    expect(positions(pages, chat)).toEqual([1, 3, 5, 7, 9, 11, 13]);
    expect(pages.snapshot(chat).loading).toBe(false);

    pages.moved(13);
    await settle();
    expect(server.reads).toHaveLength(3);

    server.add(14, 15);
    pages.moved(15);
    await server.answer(15);
    expect(server.reads.slice(3)).toEqual(["/api/tasks/task-8/events?type=task.labeled&after=13"]);
    expect(positions(pages, chat)).toEqual([1, 3, 5, 7, 9, 11, 13, 15]);
  });
});

// The server restarted on a shorter log: the events held may not be in it.
describe("a server on a shorter log", () => {
  const newest: EventsWanted = { task: "task-8", newestBefore: 101 };

  async function newestAt100() {
    const server = fakeServer([90, 95, 98, 99, 100]);
    const pages = new EventPages(server.fetch);
    pages.watch(newest, ignore);
    pages.moved(100);
    await server.answer(100);
    return { server, pages };
  }

  test("the poll's position going 100 -> 40 reads the newest page again, as of 40, showing the old events until then", async () => {
    const { server, pages } = await newestAt100();
    server.restart(10, 20, 30, 35, 40);
    pages.moved(40);
    expect(server.reads.slice(1)).toEqual(["/api/tasks/task-8/events?before=41"]);
    expect(positions(pages, newest)).toEqual([98, 99, 100]);

    await server.answer(40);
    expect(positions(pages, newest)).toEqual([30, 35, 40]);
    expect(pages.snapshot(newest)).toMatchObject({ loading: false, earlier: true, error: "" });
    pages.loadEarlier(newest);
    await server.answer(40);
    expect(positions(pages, newest)).toEqual([10, 20, 30, 35, 40]);
  });

  test("a reply read at 40, after the list has seen 100, reads the whole list again, showing the old one until it is all read", async () => {
    const server = fakeServer([1, 2, 3, 50, 60, 100]);
    const pages = new EventPages(server.fetch);
    const all: EventsWanted = { task: "task-8" };
    pages.watch(all, ignore);
    pages.moved(100);
    await server.answer(100);
    await server.answer(100);
    expect(positions(pages, all)).toEqual([1, 2, 3, 50, 60, 100]);

    pages.moved(101); // the old server's last event, before it went down
    server.restart(1, 2, 3, 4, 5, 40);
    await server.answer(40);
    expect(server.reads.slice(2)).toEqual(["/api/tasks/task-8/events?after=100", "/api/tasks/task-8/events?after=0"]);
    await server.answer(40);
    expect(positions(pages, all)).toEqual([1, 2, 3, 50, 60, 100]); // a page of the new log isn't the whole of it
    await server.answer(40);
    expect(server.reads.slice(4)).toEqual(["/api/tasks/task-8/events?after=3"]);
    expect(positions(pages, all)).toEqual([1, 2, 3, 4, 5, 40]);
    expect(pages.snapshot(all)).toMatchObject({ loading: false, error: "" });
  });

  test("a read in flight across the reset is thrown away, and the first page read after it", async () => {
    const { server, pages } = await newestAt100();
    pages.moved(101);
    pages.moved(40);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks/task-8/events?after=100"]);
    server.restart(10, 20, 30, 35, 40);
    await server.answer(101); // as if from the old log: it is thrown away whatever it holds
    expect(positions(pages, newest)).toEqual([98, 99, 100]);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks/task-8/events?before=41"]);
    await server.answer(40);
    expect(positions(pages, newest)).toEqual([30, 35, 40]);
  });

  test("after the reset the new log is followed as usual, with no reset again", async () => {
    const { server, pages } = await newestAt100();
    server.restart(10, 20, 30, 35, 40);
    pages.moved(40);
    await server.answer(40);

    pages.moved(40);
    await settle();
    expect(server.reads).toHaveLength(2);

    server.add(41);
    pages.moved(41);
    await server.answer(41);
    await settle();
    expect(server.reads.slice(2)).toEqual(["/api/tasks/task-8/events?after=40"]);
    expect(positions(pages, newest)).toEqual([30, 35, 40, 41]);
  });
});
