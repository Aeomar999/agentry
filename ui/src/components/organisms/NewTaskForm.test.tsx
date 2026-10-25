import { afterEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { api, ApiError } from "@/api/client";
import type { Agent, TeamTemplate } from "@/api/types";
import { NewTaskForm } from "./NewTaskForm";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));

// An install with only the Software team, and the Visionaries template that turns Ideas on.
const AGENTS = [{ name: "coordinator", team: "", coordinates: true, template: "", column: "" }] as unknown as Agent[];
const TEMPLATES: TeamTemplate[] = [
  {
    id: "visionaries",
    team: "Visionaries",
    description: "",
    features: ["ideas"],
    userMade: false,
    agents: [
      {
        name: "visionary",
        description: "",
        coordinates: true,
        prompt: "",
        engine: "claude",
        model: "",
        profile: "read-only",
        color: "orange",
        column: "",
        resume: "",
      },
    ],
  },
];
const READS: Record<string, unknown> = { "/api/agents": AGENTS, "/api/teams/templates": TEMPLATES, "/api/tasks": [] };
const RESOURCES = Object.fromEntries(
  Object.entries(READS).map(([path, data]) => [path, { data, error: "", loading: false, reload: async () => {} }]),
);
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => RESOURCES[path] ?? { data: undefined, error: "", loading: false, reload: async () => {} },
}));

describe("NewTaskForm: asked for the Visionaries, whose profile policy.toml lacks", () => {
  test("after the refusal only the offer shows, not the banner that was refused", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError("the policy lacks read-mostly", 409, { missingProfiles: ["read-mostly"] }));
    render(<NewTaskForm team="Visionaries" onCreated={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("alert").textContent).toContain("You have no Visionaries team yet");

    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Add the Visionaries team" })));
    const [offer, ...others] = screen.getAllByRole("alert");
    expect(others).toEqual([]);
    expect(offer.textContent).toContain("The Visionaries team needs the read-mostly profile");
    expect(screen.queryByRole("button", { name: "Add the Visionaries team" })).toBeNull();
    expect(screen.getByRole("button", { name: "Add the profile and the team" })).toBeTruthy();
  });
});

// A team from your template based on Visionaries is the Visionaries team;
// one from your template with no base isn't.
describe("NewTaskForm: a team from a template of yours", () => {
  const lead = (name: string, team: string, template: string) =>
    ({ name, team, coordinates: true, template, column: "" }) as unknown as Agent;
  const yours = (id: string, team: string, basedOn?: string): TeamTemplate => ({
    ...TEMPLATES[0],
    id,
    team,
    userMade: true,
    basedOn,
    features: basedOn ? ["ideas"] : [],
  });
  const agents = RESOURCES["/api/agents"];
  const templates = RESOURCES["/api/teams/templates"];
  afterEach(() => {
    agents.data = AGENTS;
    templates.data = TEMPLATES;
  });

  test("asks the Visionaries when the template is based on theirs", () => {
    agents.data = [...AGENTS, lead("dream-lead", "Dreamers", "dreams")];
    templates.data = [...TEMPLATES, yours("dreams", "Dreams", "visionaries")];
    render(<NewTaskForm team="Dreamers" onCreated={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("textbox", { name: "Theme or question" })).toBeTruthy();
  });

  test("is a plain task when the template has no base", () => {
    agents.data = [...AGENTS, lead("our-lead", "Ours", "ours")];
    templates.data = [...TEMPLATES, yours("ours", "Ours")];
    render(<NewTaskForm team="Ours" onCreated={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("textbox", { name: "The task" })).toBeTruthy();
  });

  // The templates list leaves out one you removed; the lead's base still says.
  test("asks the Visionaries when the template is removed and its base was theirs", () => {
    agents.data = [...AGENTS, { ...lead("dreamer", "Dreamers", "dreams"), base: "visionaries" }];
    render(<NewTaskForm team="Dreamers" onCreated={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("textbox", { name: "Theme or question" })).toBeTruthy();
  });
});
