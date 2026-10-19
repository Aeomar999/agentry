import { describe, expect, test, vi } from "vitest";
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
    agents: [{ name: "visionary", description: "", coordinates: true }],
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
