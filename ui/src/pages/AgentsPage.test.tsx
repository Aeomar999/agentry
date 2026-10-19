import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { api, ApiError } from "@/api/client";
import type { TeamTemplate } from "@/api/types";
import { AddTeam } from "./AgentsPage";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));

const template = (team: string): TeamTemplate => ({
  id: team.toLowerCase(),
  team,
  description: `The ${team} team.`,
  features: [],
  agents: [
    {
      name: `${team.toLowerCase()}-lead`,
      description: "",
      coordinates: true,
      prompt: "",
      engine: "claude",
      model: "",
      profile: "read-only",
      color: "orange",
      column: "",
    },
  ],
});
const TEMPLATES = [template("Software"), template("Writing"), template("Visionaries")];

const refused = new ApiError("the policy lacks read-mostly", 409, { missingProfiles: ["read-mostly"] });

beforeEach(() => {
  vi.mocked(api).mockReset();
  Element.prototype.scrollIntoView = vi.fn();
});

describe("AddTeam: a team refused for its profiles", () => {
  test("the offer shows in the refused team's card, scrolled into view", async () => {
    vi.mocked(api).mockRejectedValueOnce(refused);
    render(<AddTeam templates={TEMPLATES} existing={[]} policyHash="h1" onBuild={() => {}} onAdded={() => {}} />);
    const card = screen.getByRole("group", { name: "Visionaries" });
    await act(async () => fireEvent.click(within(card).getByRole("button", { name: "Add this team" })));

    const offer = within(card).getByRole("alert");
    expect(offer.textContent).toContain("The Visionaries team needs the read-mostly profile");
    expect(screen.getAllByRole("alert")).toEqual([offer]); // in no other card, nor above them all
    expect(within(offer).getByRole("button", { name: "Add the profile and the team" })).toBeTruthy();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
  });

  test("its button adds the profile with the team, against the policy the page saw", async () => {
    const onAdded = vi.fn();
    vi.mocked(api).mockRejectedValueOnce(refused).mockResolvedValueOnce({});
    render(<AddTeam templates={TEMPLATES} existing={[]} policyHash="h1" onBuild={() => {}} onAdded={onAdded} />);
    const card = screen.getByRole("group", { name: "Visionaries" });
    await act(async () => fireEvent.click(within(card).getByRole("button", { name: "Add this team" })));
    await act(async () => fireEvent.click(within(card).getByRole("button", { name: "Add the profile and the team" })));

    expect(vi.mocked(api).mock.calls[1]).toEqual(["POST", "/api/teams", { template: "Visionaries", addMissingProfiles: true, hash: "h1" }]);
    expect(onAdded).toHaveBeenCalledWith("Visionaries");
  });
});
