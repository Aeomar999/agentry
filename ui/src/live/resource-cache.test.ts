import { describe, expect, test } from "vitest";
import { ResourceCache } from "./resource-cache";

// A fake server: each read waits until the test answers it, at the log
// position the test says, so the test decides what lands mid-read.
function fakeServer() {
  const pending: { path: string; answer: (position: number, data?: unknown) => Promise<void>; fail: (message: string) => Promise<void> }[] =
    [];
  const reads: string[] = [];
  const fetch = (path: string) =>
    new Promise<{ data: unknown; position: number }>((resolve, reject) => {
      reads.push(path);
      pending.push({
        path,
        answer: async (position, data = `${path}@${position}`) => {
          resolve({ data, position });
          await settle();
        },
        fail: async (message) => {
          reject(new Error(message));
          await settle();
        },
      });
    });
  // answer answers every read in flight, at one position.
  const answer = async (position: number) => {
    for (const read of pending.splice(0)) await read.answer(position);
  };
  const readsOf = (path: string) => reads.filter((read) => read === path).length;
  return { fetch, pending, reads, answer, readsOf };
}

const settle = () => new Promise((resume) => setTimeout(resume, 0));
const ignore = () => {};

// The tasks page as it mounts: the shell and the page share the tasks and
// the agents, and the shell shows the questions too.
function showTasksPage(cache: ResourceCache) {
  return [
    cache.watch("/api/tasks", ignore),
    cache.watch("/api/questions", ignore),
    cache.watch("/api/agents", ignore),
    cache.watch("/api/tasks", ignore),
    cache.watch("/api/agents", ignore),
  ];
}

describe("the page's reads (test 9)", () => {
  test("a refresh reads each path once, and the poll's jump to the newest position reads nothing again", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    showTasksPage(cache);
    expect(server.reads.sort()).toEqual(["/api/agents", "/api/questions", "/api/tasks"]);

    cache.moved(40); // /api/changes?after=0 answers at once, while the reads are out
    await server.answer(40);
    cache.moved(40); // and again, had it answered after them
    await settle();
    expect(server.reads).toHaveLength(3);
  });

  test("one event reads each shown path once more", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    showTasksPage(cache);
    cache.moved(40);
    await server.answer(40);

    cache.moved(41);
    await server.answer(41);
    expect(["/api/tasks", "/api/questions", "/api/agents"].map(server.readsOf)).toEqual([2, 2, 2]);
  });

  test("events arriving during a read give one more read, not one each, and none is aborted", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore);
    await server.answer(40);

    cache.moved(41);
    expect(server.readsOf("/api/tasks")).toBe(2);
    for (let position = 42; position <= 50; position++) cache.moved(position);
    expect(server.readsOf("/api/tasks")).toBe(2); // still the one read, not aborted for another
    await server.answer(41);
    expect(server.readsOf("/api/tasks")).toBe(3); // it came back older than the log
    await server.answer(50);
    expect(server.readsOf("/api/tasks")).toBe(3);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@50");
  });

  test("a read that comes back as fresh as the log isn't read again", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore);
    await server.answer(40);
    cache.moved(41);
    cache.moved(42);
    await server.answer(42); // the server read it after both events
    expect(server.readsOf("/api/tasks")).toBe(2);
  });

  test("every watcher of a path is told when its data comes", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    const told: string[] = [];
    cache.watch("/api/tasks", () => told.push("shell"));
    cache.watch("/api/tasks", () => told.push("page"));
    await server.answer(40);
    expect(told).toEqual(["shell", "page"]);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "" });
  });
});

describe("the cache", () => {
  test("a path nobody shows any more is not read again, and is forgotten", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    const stop = cache.watch("/api/tasks/task-1", ignore);
    await server.answer(40);
    stop();
    cache.moved(41);
    expect(server.readsOf("/api/tasks/task-1")).toBe(1);
    expect(cache.snapshot("/api/tasks/task-1").data).toBeUndefined();
  });

  test("a path given up while it is read is forgotten when the read returns, and not read again", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    const stop = cache.watch("/api/tasks/task-1", ignore);
    stop();
    cache.moved(41);
    await server.answer(40);
    expect(server.readsOf("/api/tasks/task-1")).toBe(1);
    expect(cache.snapshot("/api/tasks/task-1").data).toBeUndefined();
  });

  test("showing a path again while it is still read shares that read", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore)(); // React's strict mode mounts twice
    cache.watch("/api/tasks", ignore);
    await server.answer(40);
    expect(server.readsOf("/api/tasks")).toBe(1);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@40");
  });

  test("reload reads again at once, or once more after the read in flight", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/questions", ignore);
    await server.answer(40);
    cache.reload("/api/questions");
    expect(server.readsOf("/api/questions")).toBe(2);
    cache.reload("/api/questions");
    cache.reload("/api/questions");
    expect(server.readsOf("/api/questions")).toBe(2);
    await server.answer(40);
    expect(server.readsOf("/api/questions")).toBe(3);
    await server.answer(40);
    expect(server.readsOf("/api/questions")).toBe(3);
  });

  test("a failed read shows its error, keeps the last data, and is tried again when the log moves", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore);
    await server.answer(40);
    cache.moved(41);
    await server.pending.splice(0)[0].fail("database is locked");
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "database is locked" });
    expect(server.readsOf("/api/tasks")).toBe(2); // not retried in a loop

    cache.moved(42);
    await server.answer(42);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@42", error: "" });
  });

  test("each path has its own data: a new path shows nothing until it is read", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks/task-1", ignore);
    await server.answer(40);
    cache.watch("/api/tasks/task-2", ignore);
    expect(cache.snapshot("/api/tasks/task-2").data).toBeUndefined();
    await server.answer(40);
    expect(cache.snapshot("/api/tasks/task-2").data).toBe("/api/tasks/task-2@40");
    expect(cache.snapshot("/api/tasks/task-1").data).toBe("/api/tasks/task-1@40");
  });
});
