import { describe, expect, test } from "vitest";
import { ResourceCache } from "./resource-cache";

// A fake server: each read waits until the test answers it, at the log
// position the test says, so the test decides what lands mid-read.
function fakeServer() {
  const pending: {
    path: string;
    answer: (position: number | undefined, data?: unknown) => Promise<void>;
    fail: (message: string) => Promise<void>;
  }[] = [];
  const reads: string[] = [];
  const fetch = (path: string) =>
    new Promise<{ data: unknown; position?: number }>((resolve, reject) => {
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
  // answer answers every read in flight, at one position (none: the reply
  // didn't say).
  const answer = async (position: number | undefined) => {
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
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "", position: 40 });
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

  test("reload returns once a read started after it is shown, not the read in flight", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/agents", ignore);
    let shown = false;
    void cache.reload("/api/agents").then(() => (shown = true));
    await server.answer(40); // the first read, out before the reload
    expect(shown).toBe(false);
    expect(cache.snapshot("/api/agents").data).toBe("/api/agents@40");
    await server.pending[0].answer(41, "with the team");
    expect(shown).toBe(true);
    expect(cache.snapshot("/api/agents").data).toBe("with the team");
  });

  test("reload returns when the path is given up before it is read again", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    const stop = cache.watch("/api/agents", ignore);
    let shown = false;
    void cache.reload("/api/agents").then(() => (shown = true));
    stop();
    await server.answer(40);
    expect(shown).toBe(true);
    expect(server.readsOf("/api/agents")).toBe(1);
    await expect(cache.reload("/api/agents")).resolves.toBeUndefined(); // a path nobody shows
  });

  test("a failed read shows its error, keeps the last data, and is tried again when the log moves", async () => {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore);
    await server.answer(40);
    cache.moved(41);
    await server.pending.splice(0)[0].fail("database is locked");
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "database is locked", position: 40 });
    expect(server.readsOf("/api/tasks")).toBe(2); // not retried in a loop

    cache.moved(42);
    await server.answer(42);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@42", error: "", position: 42 });
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

describe("a reply that doesn't say its position", () => {
  // Each starts with the page live at 40 and one path shown, so the cache
  // asks for it as of 40.
  async function shownAt40() {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.moved(40);
    cache.watch("/api/tasks", ignore);
    return { server, cache };
  }

  test("a reply with no position is read once, not in a loop, and again when the log moves", async () => {
    const { server, cache } = await shownAt40();
    await server.answer(undefined);
    await settle();
    expect(server.readsOf("/api/tasks")).toBe(1);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@undefined");

    cache.moved(41);
    expect(server.readsOf("/api/tasks")).toBe(2);
  });
});

// The server restarted on a shorter log: a restored or different database,
// or a test server. Each starts with two paths shown, read at 100.
describe("a server on a shorter log", () => {
  async function shownAt100() {
    const server = fakeServer();
    const cache = new ResourceCache(server.fetch);
    cache.watch("/api/tasks", ignore);
    cache.watch("/api/agents", ignore);
    cache.moved(100);
    await server.answer(100);
    return { server, cache };
  }

  test("the poll's position going 100 -> 40 reads every path again, showing the old data until then", async () => {
    const { server, cache } = await shownAt100();
    cache.moved(40);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks", "/api/agents"]);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@100", error: "", position: 100 });

    await server.answer(40);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "", position: 40 });
    expect(cache.snapshot("/api/agents").data).toBe("/api/agents@40");
  });

  test("a reply read at 40, after the cache has seen 100, reads every path again", async () => {
    const { server, cache } = await shownAt100();
    void cache.reload("/api/tasks");
    await server.answer(40);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@100"); // the reply that told is thrown away too
    expect(server.pending.map((read) => read.path)).toEqual(["/api/agents", "/api/tasks"]);

    await server.answer(40);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@40");
    expect(cache.snapshot("/api/agents").data).toBe("/api/agents@40");
  });

  test("a read in flight across the reset is thrown away and read again, and a reload waits for that read", async () => {
    const { server, cache } = await shownAt100();
    let shown = false;
    void cache.reload("/api/tasks").then(() => (shown = true));
    cache.moved(40);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/tasks", "/api/agents"]); // the reload's read, and agents again

    await server.pending.splice(0, 1)[0].answer(101, "from the old log");
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@100", error: "", position: 100 });
    expect(shown).toBe(false);
    expect(server.pending.map((read) => read.path)).toEqual(["/api/agents", "/api/tasks"]);

    await server.answer(40);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "", position: 40 });
    expect(shown).toBe(true);
    expect(server.readsOf("/api/tasks")).toBe(3);
  });

  test("a read in flight across the reset that fails shows no error, and is read again", async () => {
    const { server, cache } = await shownAt100();
    cache.moved(101);
    cache.moved(40);
    await server.pending.splice(0, 1)[0].fail("connection refused"); // the old server going down
    expect(cache.snapshot("/api/tasks").error).toBe("");
    await server.answer(40);
    await server.answer(40);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@40", error: "", position: 40 });
  });

  test("after the reset the new log is followed as usual, with no reset again", async () => {
    const { server, cache } = await shownAt100();
    cache.moved(40);
    await server.answer(40);

    cache.moved(40);
    await settle();
    expect(server.reads).toHaveLength(4);

    cache.moved(41);
    await server.answer(41);
    await settle();
    expect(server.reads).toHaveLength(6);
    expect(cache.snapshot("/api/tasks")).toEqual({ data: "/api/tasks@41", error: "", position: 41 });
  });

  test("a reply older than the poll's position is no reset: it was read before the event", async () => {
    const { server, cache } = await shownAt100();
    cache.moved(101);
    cache.moved(102);
    await server.answer(101);
    expect(cache.snapshot("/api/tasks").data).toBe("/api/tasks@101");
    await server.answer(102);
    expect(server.reads).toHaveLength(6); // read once more for 102, not all over
  });
});
