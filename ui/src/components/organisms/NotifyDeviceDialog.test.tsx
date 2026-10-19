import { describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { PushStatus } from "@/api/types";
import { NotifyDeviceDialog } from "./NotifyDeviceDialog";

// The server's answer to GET /api/push, and what this browser can do, as
// each test sets them.
let server: PushStatus;
let inBrowser = "";
vi.mock("@/api/client", () => ({ api: () => Promise.resolve(server) }));
vi.mock("@/lib/push", () => ({
  browserReason: () => inBrowser,
  thisDevice: () => Promise.resolve(null),
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
}));

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

  test("with push on but a browser without it, the browser's reason shows and there is no button", async () => {
    server = { available: true, publicKey: "BPublicKey", subscribed: 0 };
    inBrowser = "This browser can't receive pushes here.";
    render(<NotifyDeviceDialog onClose={() => {}} />);
    expect((await screen.findByRole("note")).textContent).toBe(inBrowser);
    expect(button()).toBeNull();
  });
});
