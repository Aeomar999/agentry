import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api, ApiError, send } from "@/api/client";
import type { Agent, TeamTemplate } from "@/api/types";
import { AgentsPage } from "./AgentsPage";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn(), send: vi.fn() }));

// The page reads the agents, the templates and the policy; each is answered here by its path.
const LEAD = { name: "lead", description: "", coordinates: true, team: "", workers: [] } as unknown as Agent;
const PAPERS = {
  id: "papers",
  team: "Papers",
  description: "",
  features: [],
  userMade: true,
  savedAt: new Date().toISOString(),
  agents: [],
} as TeamTemplate;
const READ: Record<string, unknown> = { "/api/agents": [LEAD], "/api/teams/templates": [PAPERS], "/api/policy": { hash: "h1" } };
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => ({ data: READ[path], error: "", position: 1, reload: async () => {} }),
}));

// Each request is held until the test lets it go, with a failure, so the dialog stays to be tried again, or with success.
let fail = (_: Error) => {};
let succeed = () => {};
const held = () =>
  new Promise<never>((resolve, reject) => {
    fail = reject;
    succeed = () => resolve(undefined as never);
  });

const dialog = () => screen.queryByRole("dialog", { name: "Add a team" });
const scrim = () => dialog()!.parentElement!;
const escape = () => act(async () => fireEvent.keyDown(window, { key: "Escape" }));
const outside = () => act(async () => fireEvent.mouseDown(scrim()));
const click = (button: HTMLElement) => act(async () => fireEvent.click(button));
const papers = () => within(dialog()!).getByRole("group", { name: "Papers" });

beforeEach(() => {
  vi.mocked(api).mockReset();
  vi.mocked(send).mockReset();
  Element.prototype.scrollIntoView = vi.fn();
  render(
    <MemoryRouter>
      <AgentsPage />
    </MemoryRouter>,
  );
});

const starts: Record<string, () => Promise<void>> = {
  "an add": async () => {
    vi.mocked(api).mockReturnValueOnce(held());
    await click(within(papers()).getByRole("button", { name: "Add this team, Papers" }));
  },
  "a delete": async () => {
    vi.mocked(api).mockReturnValueOnce(held());
    await click(within(papers()).getByRole("button", { name: "Delete template, Papers" }));
    await click(within(papers()).getByRole("button", { name: "Delete" }));
  },
  "an import": async () => {
    vi.mocked(send).mockReturnValueOnce(held());
    const file = new File(["{}"], "papers.json", { type: "application/json" });
    await act(async () => fireEvent.change(screen.getByTestId("template-file"), { target: { files: [file] } }));
  },
};

describe("AgentsPage: Add a team", () => {
  for (const [request, start] of Object.entries(starts)) {
    test(`while ${request} runs, Esc and a click outside leave it open; once it ends, both close it`, async () => {
      await click(screen.getByRole("button", { name: "Add a team" }));
      await start();
      await escape();
      await outside();
      expect(dialog()).not.toBeNull();

      await act(async () => fail(new ApiError("refused", 409)));
      await outside();
      expect(dialog()).toBeNull();
      await click(screen.getByRole("button", { name: "Add a team" }));
      await escape();
      expect(dialog()).toBeNull();
    });
  }

  // The new list says nothing runs as it opens, so this holds even if the closing one never said its add ended; AddTeam.test.tsx checks that.
  test("once an add succeeds and closes it, Esc and a click outside close it when it opens again", async () => {
    await click(screen.getByRole("button", { name: "Add a team" }));
    vi.mocked(api).mockReturnValueOnce(held());
    await click(within(papers()).getByRole("button", { name: "Add this team, Papers" }));
    await act(async () => succeed());
    expect(dialog()).toBeNull();

    await click(screen.getByRole("button", { name: "Add a team" }));
    await escape();
    expect(dialog()).toBeNull();
    await click(screen.getByRole("button", { name: "Add a team" }));
    await outside();
    expect(dialog()).toBeNull();
  });
});
