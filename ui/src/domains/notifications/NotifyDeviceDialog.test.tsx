import { beforeEach, describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { PushStatus } from "@/api/types";
import { NotifyDeviceDialog } from "./NotifyDeviceDialog";

// The server's answer to GET /api/push, what this browser can do and the
// subscription it already has, as each test sets them.
let server: PushStatus;
let inBrowser = "";
let existing: PushSubscription | null = null;
const api = vi.fn((method: string, _path: string, _body?: unknown) => Promise.resolve(method === "GET" ? server : {}));
vi.mock("@/api/client", () => ({ api: (method: string, path: string, body?: unknown) => api(method, path, body) }));
vi.mock("@/domains/notifications/push", async (actual) => ({
  ...(await actual<typeof import("@/domains/notifications/push")>()),
  browserReason: () => inBrowser,
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
}));

beforeEach(() => {
  api.mockClear();
  existing = null;
  Object.defineProperty(navigator, "serviceWorker", {
    configurable: true,
    value: { getRegistration: () => Promise.resolve({ pushManager: { getSubscription: () => Promise.resolve(existing) } }) },
  });
});

// subscribedWith is a browser subscription made with the key in bytes.
function subscribedWith(bytes: number[]) {
  const json = { endpoint: "https://push.example/device", keys: { p256dh: "BKey", auth: "secret" } };
  return {
    endpoint: json.endpoint,
    options: { applicationServerKey: new Uint8Array(bytes).buffer },
    toJSON: () => json,
    unsubscribe: vi.fn(() => Promise.resolve(true)),
  } as unknown as PushSubscription & { unsubscribe: ReturnType<typeof vi.fn> };
}

const NO_HTTPS = "Push needs an https address. Start Agentry with -public-url https://… so phones can reach it.";
const button = () => screen.queryByRole("button", { name: "Notify me on this device" });

describe("Notifications on this device", () => {
  test("with push off at the server, its reason shows and there is no button", async () => {
    server = { available: false, reason: NO_HTTPS, subscribed: 0 };
    inBrowser = "";
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect((await screen.findByRole("note")).textContent).toBe(NO_HTTPS);
    expect(button()).toBeNull();
  });

  test("with push on, the button shows", async () => {
    server = { available: true, publicKey: "BPublicKey", subscribed: 0 };
    inBrowser = "";
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect(await screen.findByRole("button", { name: "Notify me on this device" })).toBeTruthy();
    expect(screen.queryByRole("note")).toBeNull();
  });

  test("a subscription made with another key is dropped in the browser and the button shows", async () => {
    server = { available: true, publicKey: "AQID", subscribed: 0 }; // bytes 1, 2, 3
    inBrowser = "";
    const old = subscribedWith([9, 9, 9]);
    existing = old;
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect(await screen.findByRole("button", { name: "Notify me on this device" })).toBeTruthy();
    expect(old.unsubscribe).toHaveBeenCalled();
    expect(api).not.toHaveBeenCalledWith("POST", expect.anything(), expect.anything());
  });

  test("a subscription made with the server's key is sent again and shows as notified", async () => {
    server = { available: true, publicKey: "AQID", subscribed: 0 };
    inBrowser = "";
    const mine = subscribedWith([1, 2, 3]);
    existing = mine;
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect(await screen.findByText("This device is notified when something waits for you.")).toBeTruthy();
    expect(api).toHaveBeenCalledWith("POST", "/api/push/subscriptions", mine.toJSON());
    expect(mine.unsubscribe).not.toHaveBeenCalled();
  });

  test("with push on but a browser without it, the browser's reason shows and there is no button", async () => {
    server = { available: true, publicKey: "BPublicKey", subscribed: 0 };
    inBrowser = "This browser can't receive pushes here.";
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect((await screen.findByRole("note")).textContent).toBe(inBrowser);
    expect(button()).toBeNull();
  });
});
