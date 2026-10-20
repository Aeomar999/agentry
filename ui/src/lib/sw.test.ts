import { describe, expect, test, vi } from "vitest";
import source from "../../public/sw.js?raw";

// The service worker (public/sw.js) runs here against a stand-in for its
// global scope: each test gets the handlers it adds and sees what they ask
// the browser for.

const origin = "https://agentry.example";

type Handler = (event: Record<string, unknown>) => void;
type Window = { url: string; focus: () => Promise<void>; navigate: (url: string) => Promise<void> };

function startWorker(windows: Window[] = []) {
  const handlers: Record<string, Handler> = {};
  const scope = {
    location: new URL(origin + "/sw.js"),
    addEventListener: (type: string, handler: Handler) => (handlers[type] = handler),
    skipWaiting: vi.fn(),
    registration: { showNotification: vi.fn(() => Promise.resolve()) },
    clients: { claim: vi.fn(), matchAll: vi.fn(() => Promise.resolve(windows)), openWindow: vi.fn(() => Promise.resolve(null)) },
  };
  new Function("self", source)(scope);
  return { handlers, scope };
}

// dispatch hands handler an event and waits for what it gave waitUntil.
async function dispatch(handler: Handler, event: Record<string, unknown>) {
  let kept: Promise<unknown> = Promise.resolve();
  handler({ ...event, waitUntil: (promise: Promise<unknown>) => (kept = promise) });
  await kept;
}

function pushOf(json: () => unknown) {
  return { data: { json } };
}

function clickOn(url: unknown) {
  return { notification: { close: vi.fn(), data: { url } } };
}

describe("a push", () => {
  test("shows its title and body, tagged, with the page to open", async () => {
    const { handlers, scope } = startWorker();
    const message = {
      title: "Agentry",
      body: "task-4: Waiting for you to approve the push",
      task: "task-4",
      url: "/tasks/task-4",
      tag: "ab12",
    };
    await dispatch(
      handlers.push,
      pushOf(() => message),
    );
    expect(scope.registration.showNotification).toHaveBeenCalledWith("Agentry", {
      body: message.body,
      tag: "ab12",
      data: { url: "/tasks/task-4" },
    });
  });

  test("that can't be read still shows a plain notification", async () => {
    for (const event of [
      { data: null },
      pushOf(() => {
        throw new SyntaxError("not JSON");
      }),
      pushOf(() => ({ title: 4, body: ["no"] })),
    ]) {
      const { handlers, scope } = startWorker();
      await dispatch(handlers.push, event);
      expect(scope.registration.showNotification).toHaveBeenCalledWith("Agentry", { body: "", tag: undefined, data: { url: "" } });
    }
  });
});

describe("tapping a notification", () => {
  test("opens its page when no window of the app is open", async () => {
    const { handlers, scope } = startWorker();
    const click = clickOn("/tasks/task-4");
    await dispatch(handlers.notificationclick, click);
    expect(click.notification.close).toHaveBeenCalled();
    expect(scope.clients.openWindow).toHaveBeenCalledWith(origin + "/tasks/task-4");
  });

  test("shows its page in the app's open window", async () => {
    const open: Window = { url: origin + "/", focus: vi.fn(() => Promise.resolve()), navigate: vi.fn(() => Promise.resolve()) };
    const { handlers, scope } = startWorker([open]);
    await dispatch(handlers.notificationclick, clickOn("/tasks/task-4"));
    expect(open.focus).toHaveBeenCalled();
    expect(open.navigate).toHaveBeenCalledWith(origin + "/tasks/task-4");
    expect(scope.clients.openWindow).not.toHaveBeenCalled();
  });

  test("opens only Agentry, whatever address the push names", async () => {
    for (const url of [
      "https://elsewhere.example/tasks/task-4",
      "//elsewhere.example/x",
      "/\\elsewhere.example/x",
      "javascript:alert(1)",
      "tasks/task-4",
      7,
      undefined,
    ]) {
      const { handlers, scope } = startWorker();
      await dispatch(handlers.notificationclick, clickOn(url));
      expect(scope.clients.openWindow, String(url)).toHaveBeenCalledWith(origin + "/");
    }
  });
});
