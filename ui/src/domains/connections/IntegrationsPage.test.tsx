import { useSyncExternalStore } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api } from "@/api/client";
import type { InstallCommands } from "@/api/types";
import { IntegrationsPage } from "./IntegrationsPage";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));

// The connections list is held here. Like a real read, each reload answers
// fresh objects for the same connections, so a page that reloads whenever
// it sees a new object would reload for ever: reload stops answering fresh
// ones after MAX_READS, so such a page fails the test instead of hanging it.
const MAX_READS = 20;
let connections: object[] = [];
const watchers = new Set<() => void>();
const reload = vi.fn(async () => {
  if (reload.mock.calls.length > MAX_READS) return;
  connections = connections.map((c) => ({ ...c }));
  watchers.forEach((changed) => changed());
});
const watch = (changed: () => void) => (watchers.add(changed), () => watchers.delete(changed));
vi.mock("@/live/use-resource", () => ({
  useResource: () => ({ data: { connections: useSyncExternalStore(watch, () => connections) }, error: "", position: 1, reload }),
}));

const LAPTOP = { id: "c1", host: "laptop", user: "dev", os: "linux", madeAt: "2026-10-30T09:00:00Z" };
const STUDIO = { id: "c2", host: "studio", user: "dev", os: "darwin", madeAt: "2026-10-30T10:00:00Z" };
const COMMANDS: InstallCommands = {
  command: "curl … | sh",
  windowsCommand: "irm … | iex",
  address: "http://agentry.test",
  addresses: [],
  expires: "2026-10-30T11:00:00Z",
};

const join = (connection: object) =>
  act(async () => {
    connections = [...connections, connection];
    watchers.forEach((changed) => changed());
  });

beforeEach(() => {
  connections = [LAPTOP];
  reload.mockClear();
  vi.mocked(api).mockReset();
});

describe("IntegrationsPage", () => {
  test("reloads the connections once when a machine joins, and shows it", async () => {
    vi.mocked(api).mockResolvedValueOnce(COMMANDS);
    render(
      <MemoryRouter>
        <IntegrationsPage />
      </MemoryRouter>,
    );
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Make an install command" })));
    expect(reload).not.toHaveBeenCalled();

    await join(STUDIO);

    expect(reload).toHaveBeenCalledTimes(1);
    expect(screen.getByText("studio is connected")).toBeTruthy();
    expect(screen.getByText("studio")).toBeTruthy();
    expect(screen.getByText("2 connected")).toBeTruthy();
  });
});
