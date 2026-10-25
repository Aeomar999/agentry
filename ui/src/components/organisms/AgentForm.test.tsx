import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api } from "@/api/client";
import type { Agent } from "@/api/types";
import { AgentForm } from "./AgentForm";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));
vi.mock("@/components/organisms/Toasts", async (actual) => ({
  ...(await actual<typeof import("@/components/organisms/Toasts")>()),
  useToast: () => vi.fn(),
}));
// Monaco doesn't run in jsdom: the prompt is a plain box here.
vi.mock("@/components/molecules/CodeViewer", () => ({
  CodeViewer: ({ text, onChange }: { text: string; onChange: (text: string) => void }) => (
    <textarea aria-label="Prompt" value={text} onChange={(e) => onChange(e.target.value)} />
  ),
}));
const READS: Record<string, unknown> = {
  "/api/agents": [],
  "/api/config": { engines: ["claude", "codex"], workers: ["local"] },
  "/api/policy": { hash: "h1", defaultProfile: "developer", useForEveryone: "", profiles: [], defaults: [] },
};
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => ({ data: READS[path], error: "", loading: false, reload: async () => {} }),
}));

const reviewer: Agent = {
  name: "reviewer",
  description: "Checks the work.",
  prompt: "You review.",
  engine: "claude",
  model: "",
  profile: "",
  coordinates: false,
  team: "",
  template: "",
  workers: [],
  color: "",
  column: "review",
  resume: "",
  knowledge: "",
};

const show = (agent?: Agent) =>
  render(
    <MemoryRouter>
      <AgentForm agent={agent} />
    </MemoryRouter>,
  );
const startsEachTurn = () => screen.getByLabelText("Starts each turn") as HTMLSelectElement;
const hint = (starts: string) => screen.getByText(new RegExp(`^${starts.replace(/[.()]/g, "\\$&")}`));

describe("AgentForm: how the agent starts each turn", () => {
  beforeEach(() => vi.mocked(api).mockReset());

  test("By its work says what it comes to for this agent, following its column as it changes", () => {
    show(reviewer);
    expect(startsEachTurn().value).toBe("");
    expect(hint("Fresh, since its work counts as review. A reviewer starts fresh and gets a compact handoff")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Its work counts as"), { target: { value: "ship" } });
    expect(hint("Going on with its session, since its work counts as shipping.")).toBeTruthy();
  });

  test("a choice has its own hint and is saved with the agent", async () => {
    vi.mocked(api).mockResolvedValue({});
    show(reviewer);
    fireEvent.change(startsEachTurn(), { target: { value: "resume" } });
    expect(hint("Goes on with its own session from its last turn on the task")).toBeTruthy();
    fireEvent.change(startsEachTurn(), { target: { value: "fresh" } });
    expect(hint("Starts a new session every turn, briefed from scratch.")).toBeTruthy();

    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save" })));
    expect(api).toHaveBeenCalledWith("PUT", "/api/agents/reviewer", { ...reviewer, resume: "fresh" });
  });

  test("a new agent starts by its work, which is building until its column says otherwise", async () => {
    vi.mocked(api).mockResolvedValue({});
    show();
    expect(startsEachTurn().value).toBe("");
    expect(hint("Going on with its session, since its work counts as building.")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "scribe" } });
    fireEvent.change(screen.getByLabelText("Prompt"), { target: { value: "You write." } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Add agent" })));
    expect(api).toHaveBeenCalledWith("POST", "/api/agents", expect.objectContaining({ name: "scribe", column: "", resume: "" }));
  });
});
