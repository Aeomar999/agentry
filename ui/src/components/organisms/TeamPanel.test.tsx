import { useSyncExternalStore } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api, ApiError } from "@/api/client";
import type { Agent, Policy, TeamTemplate, TemplateAgent } from "@/api/types";
import { AgentsPage } from "@/pages/AgentsPage";

// The New team panel on the Agents page, with the API answered here: the
// reads from a little store the page's reloads can change, the team's
// POST from a mock.

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));

// Monaco doesn't run in jsdom: the prompt is a plain box here.
vi.mock("@/components/molecules/CodeViewer", () => ({
  CodeViewer: ({ text, onChange, readOnly }: { text: string; onChange: (text: string) => void; readOnly?: boolean }) => (
    <textarea value={text} readOnly={readOnly} onChange={(e) => onChange(e.target.value)} />
  ),
}));

const reads: Record<string, unknown> = {};
let afterReload: Record<string, unknown> = {}; // what a reload of a path reads next
const watchers = new Set<() => void>();
const changed = () => watchers.forEach((watcher) => watcher());

vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => {
    const data = useSyncExternalStore(
      (watcher) => (watchers.add(watcher), () => watchers.delete(watcher)),
      () => reads[path],
    );
    const reload = async () => {
      if (path in afterReload) reads[path] = afterReload[path];
      changed();
    };
    return { data, error: "", position: 1, reload };
  },
}));

function agent(name: string, team: string, given: Partial<Agent> = {}): Agent {
  return {
    name,
    description: "",
    prompt: "",
    engine: "claude",
    model: "",
    profile: "",
    coordinates: false,
    team,
    template: "",
    workers: [],
    color: "",
    column: "",
    knowledge: "",
    ...given,
  };
}

const ROSTER = [agent("tech-lead", "", { coordinates: true }), agent("reviewer", "", { column: "review" })];

const member = (name: string, given: Partial<TemplateAgent> = {}): TemplateAgent => ({
  name,
  description: `The ${name}.`,
  coordinates: false,
  prompt: `You are the ${name}.`,
  engine: "claude",
  model: "",
  profile: "",
  color: "",
  column: "",
  ...given,
});

const WRITING: TeamTemplate = {
  id: "writing",
  team: "Writing",
  description: "Writes docs.",
  features: [],
  agents: [
    member("editor", { coordinates: true, prompt: "", profile: "developer", color: "violet" }),
    member("writer", { engine: "codex", model: "gpt-5", profile: "developer", color: "blue", column: "review" }),
  ],
};
const IDEAS: TeamTemplate = {
  id: "visionaries",
  team: "Visionaries",
  description: "Suggests features.",
  features: ["ideas"],
  agents: [member("visionary", { coordinates: true, profile: "read-mostly" }), member("skeptic", { profile: "read-mostly" })],
};

const POLICY = {
  hash: "h1",
  defaultProfile: "developer",
  useForEveryone: "",
  profiles: [{ name: "developer", why: "", otherwise: "ask", rules: [] }],
  defaults: [{ name: "read-mostly", state: "missing", text: "" }],
} as unknown as Policy;

beforeEach(() => {
  vi.mocked(api).mockReset();
  Element.prototype.scrollIntoView = vi.fn();
  Object.assign(reads, {
    "/api/agents": ROSTER,
    "/api/teams/templates": [WRITING, IDEAS],
    "/api/policy": POLICY,
    "/api/config": { engines: ["claude", "codex"] },
  });
  afterReload = {};
});

const openAt = (address: string) =>
  render(
    <MemoryRouter initialEntries={[address]}>
      <AgentsPage />
    </MemoryRouter>,
  );

const panel = () => screen.getByRole("dialog", { name: "New team" });
const teamName = () => within(panel()).getByLabelText("Team name") as HTMLInputElement;
const rowButton = (name: string) => within(panel()).getByRole("button", { name: new RegExp(`^${name}(,|$)`) });
const field = (label: string) => within(panel()).getByLabelText(label) as HTMLInputElement;
const create = () => act(async () => fireEvent.click(within(panel()).getByRole("button", { name: "Create team" })));
const summary = () => document.getElementById("team-summary")!;
const sent = () => vi.mocked(api).mock.calls.map(([method, path, body]) => [method, path, body]);
const addTeam = () => screen.getByRole("button", { name: "Add a team" });
// said checks the panel's live region says text, which it does a frame after the change.
const said = async (text: string) => expect((await within(panel()).findByText(text)).getAttribute("aria-live")).toBe("polite");
const liveRegion = () => panel().querySelector("p[aria-live].sr-only")!;

describe("New team: Blank", () => {
  test("Build a team in Add a team opens the panel with one coordinator", () => {
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Add a team" }));
    fireEvent.click(screen.getByRole("button", { name: "Build a team" }));

    expect(screen.queryByRole("dialog", { name: "Add a team" })).toBeNull();
    expect(teamName().value).toBe("");
    expect(document.activeElement).toBe(teamName());
    expect(within(panel()).getByRole("radio", { name: "Blank" }).getAttribute("aria-checked")).toBe("true");
    expect(within(panel()).queryByLabelText("Template")).toBeNull();
    expect(rowButton("coordinator").getAttribute("aria-expanded")).toBe("true");
    expect((within(panel()).getByRole("radio", { name: "Coordinator, coordinator" }) as HTMLInputElement).checked).toBe(true);
    expect(within(panel()).getByText("Creates the team and its coordinator.")).toBeTruthy();
    expect(within(panel()).getByText("Empty: it uses Agentry's coordinator prompt for a team of these agents.")).toBeTruthy();
  });

  test("creates the team as typed, the coordinator on the default prompt", async () => {
    vi.mocked(api).mockResolvedValue({});
    openAt("/agents?team=new");
    fireEvent.change(teamName(), { target: { value: " Docs " } });
    await create();

    expect(sent()).toEqual([
      [
        "POST",
        "/api/teams",
        {
          team: "Docs",
          agents: [
            {
              name: "coordinator",
              description: "",
              coordinates: true,
              prompt: "",
              engine: "claude",
              model: "",
              profile: "",
              color: "",
              column: "",
            },
          ],
        },
      ],
    ]);
  });

  test("the page behind is inert while the panel is open, and Close closes it", () => {
    const { container } = openAt("/agents?team=new");
    expect(container.inert).toBe(true);
    fireEvent.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(container.inert).toBe(false);
  });
});

describe("New team: focus and scroll", () => {
  test("Close puts focus on Add a team, whose dialog opened the panel", () => {
    openAt("/agents");
    fireEvent.click(addTeam());
    fireEvent.click(screen.getByRole("button", { name: "Build a team" }));
    fireEvent.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(document.activeElement).toBe(addTeam());
  });

  test("Cancel puts focus on Add a team, from a link too", () => {
    openAt("/agents?team=new");
    fireEvent.click(within(panel()).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(document.activeElement).toBe(addTeam());
  });

  test("creating the team puts focus on Add a team", async () => {
    vi.mocked(api).mockResolvedValue({});
    openAt("/agents");
    fireEvent.click(addTeam());
    fireEvent.click(screen.getByRole("button", { name: "Change it first, Writing" }));
    await create();
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(document.activeElement).toBe(addTeam());
  });

  test("focus stays in Team name when the panel finishes loading", () => {
    delete reads["/api/config"];
    openAt("/agents?team=new");
    expect(within(panel()).getByText("Loading engines and profiles…")).toBeTruthy();
    act(() => {
      reads["/api/config"] = { engines: ["claude", "codex"] };
      changed();
    });
    expect(document.activeElement).toBe(teamName());
  });

  test("the page doesn't scroll under the panel", () => {
    const page = document.documentElement;
    page.style.overflow = "auto";
    openAt("/agents?team=new");
    expect(page.style.overflow).toBe("hidden");
    fireEvent.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(page.style.overflow).toBe("auto");
    page.style.overflow = "";
  });
});

describe("New team: Copy a template", () => {
  test("Change it first copies every agent, prompts included, under a numbered team name", async () => {
    vi.mocked(api).mockResolvedValue({});
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Add a team" }));
    fireEvent.click(screen.getByRole("button", { name: "Change it first, Writing" }));

    expect(teamName().value).toBe("Writing 2");
    expect(within(panel()).getByRole("radio", { name: "Copy a template" }).getAttribute("aria-checked")).toBe("true");
    expect((within(panel()).getByLabelText("Template") as HTMLSelectElement).value).toBe("writing");
    expect(within(panel()).getByText("2 agents")).toBeTruthy();

    fireEvent.click(rowButton("writer"));
    expect(rowButton("editor").getAttribute("aria-expanded")).toBe("false");
    expect(field("Name").value).toBe("writer");
    expect(field("Description").value).toBe("The writer.");
    expect(within(panel()).getByDisplayValue("You are the writer.")).toBeTruthy();
    expect((field("Engine") as unknown as HTMLSelectElement).value).toBe("codex");
    expect(field("Model").value).toBe("gpt-5");
    expect((field("Policy profile") as unknown as HTMLSelectElement).value).toBe("developer");
    expect((field("Its work counts as") as unknown as HTMLSelectElement).value).toBe("review");
    expect(within(panel()).getByRole("radio", { name: "blue" }).getAttribute("aria-checked")).toBe("true");

    await create();
    const [, , body] = sent()[0];
    expect(body).toEqual({
      team: "Writing 2",
      agents: WRITING.agents,
    });
  });

  test("picking another template after changing agents asks first, in place", async () => {
    openAt("/agents?team=new&template=writing");
    fireEvent.click(rowButton("writer"));
    fireEvent.change(field("Description"), { target: { value: "Drafts." } });
    fireEvent.change(within(panel()).getByLabelText("Template"), { target: { value: "visionaries" } });

    const question = within(panel()).getByRole("group", { name: "Replace your changed agent with Visionaries's?" });
    expect(question.textContent).toContain("your changes to writer go.");
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep mine" }));
    fireEvent.click(within(question).getByRole("button", { name: "Keep mine" }));
    expect(rowButton("writer")).toBeTruthy();

    fireEvent.change(within(panel()).getByLabelText("Template"), { target: { value: "visionaries" } });
    fireEvent.click(within(panel()).getByRole("button", { name: "Replace" }));
    await said("Replaced with Visionaries's 2 agents.");
    expect(teamName().value).toBe("Visionaries 2");
    expect(within(panel()).queryByRole("button", { name: /^writer/ })).toBeNull();
    expect(rowButton("visionary")).toBeTruthy();
  });
  test("Blank after changing agents asks first, then says the team is blank", async () => {
    openAt("/agents?team=new&template=writing");
    fireEvent.click(rowButton("writer"));
    fireEvent.change(field("Description"), { target: { value: "Drafts." } });
    fireEvent.click(within(panel()).getByRole("radio", { name: "Blank" }));
    within(panel()).getByRole("group", { name: "Replace your changed agent with a blank team?" });
    fireEvent.click(within(panel()).getByRole("button", { name: "Replace" }));

    await said("Replaced with a blank team: one agent, its coordinator.");
    expect(rowButton("coordinator")).toBeTruthy();
    expect(within(panel()).queryByRole("button", { name: /^writer/ })).toBeNull();
  });
});

describe("New team: agents", () => {
  test("Add agent opens a new row with focus in its Name; remove leaves a stub with Undo", async () => {
    openAt("/agents?team=new");
    fireEvent.click(within(panel()).getByRole("button", { name: "Add agent" }));
    expect(rowButton("New agent").getAttribute("aria-expanded")).toBe("true");
    expect(rowButton("coordinator").getAttribute("aria-expanded")).toBe("false");
    expect((document.activeElement as HTMLInputElement).value).toBe("");
    expect(document.activeElement).toBe(field("Name"));

    fireEvent.click(within(panel()).getByRole("button", { name: "Remove coordinator" }));
    const undo = within(panel()).getByRole("button", { name: "Undo, keep coordinator" });
    expect(within(panel()).getByText("coordinator removed")).toBeTruthy();
    await said("coordinator removed. Undo is next.");
    expect(document.activeElement).toBe(undo);
    fireEvent.click(undo);
    await said("coordinator is back.");
    expect(document.activeElement).toBe(rowButton("coordinator"));
    expect((within(panel()).getByRole("radio", { name: "Coordinator, coordinator" }) as HTMLInputElement).checked).toBe(true);
  });

  test("the same words twice in a row are said again: the region empties, then speaks", async () => {
    openAt("/agents?team=new");
    fireEvent.click(within(panel()).getByRole("button", { name: "Add agent" }));
    fireEvent.click(within(panel()).getByRole("button", { name: "Add agent" }));
    const [first, second] = within(panel()).getAllByRole("button", { name: "Remove New agent" });
    fireEvent.click(first);
    await said("New agent removed. Undo is next.");

    fireEvent.click(second);
    expect(liveRegion().textContent).toBe("");
    await said("New agent removed. Undo is next.");
  });

  test("the coordinator is one radio group across the rows", () => {
    openAt("/agents?team=new&template=writing");
    fireEvent.click(within(panel()).getByRole("radio", { name: "Coordinator, writer" }));
    const radios = within(panel()).getAllByRole("radio", { name: /^Coordinator, / }) as HTMLInputElement[];
    expect(radios.map((radio) => [radio.name, radio.checked])).toEqual([
      ["coordinator", false],
      ["coordinator", true],
    ]);
  });
});

describe("New team: problems", () => {
  test("an empty team name is in the summary, which takes focus, and under the field", async () => {
    openAt("/agents?team=new");
    await create();

    expect(api).not.toHaveBeenCalled();
    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't create the team: 1 thing to fix");
    expect(document.activeElement).toBe(summary());
    expect(summary().getAttribute("role")).toBeNull();
    expect(teamName().getAttribute("aria-invalid")).toBe("true");
    expect(document.getElementById(teamName().getAttribute("aria-describedby")!)!.textContent).toBe("Give the team a name.");
    expect(within(panel()).getByText("1 thing to fix, listed at the top.")).toBeTruthy();

    fireEvent.click(within(summary()).getByRole("button", { name: "Team name: give the team a name" }));
    expect(document.activeElement).toBe(teamName());
  });

  test("every problem at once: team, agents and each row's fields", async () => {
    reads["/api/teams/templates"] = [
      {
        ...WRITING,
        agents: [
          member("editor", { prompt: "" }),
          member("writer"),
          member("writer"), // copied as writer-2
          member("reviewer"), // copied as reviewer-2, since you have a reviewer
          member("Fact checker"),
          member("checker", { profile: "nope" }),
        ],
      },
      IDEAS,
    ];
    openAt("/agents?team=new&template=writing");
    fireEvent.change(teamName(), { target: { value: "software" } });
    fireEvent.click(rowButton("writer-2"));
    fireEvent.change(field("Name"), { target: { value: "writer" } });
    fireEvent.click(rowButton("reviewer-2"));
    fireEvent.change(field("Name"), { target: { value: "reviewer" } });
    await create();

    const links = within(summary())
      .getAllByRole("button")
      .map((link) => link.textContent);
    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't create the team: 7 things to fix");
    expect(links).toEqual([
      "Team name: you have a team called Software already",
      "Agents: pick one agent to coordinate",
      "editor: write a prompt",
      "writer (second one): another agent in this team is called writer",
      "reviewer: you have an agent called reviewer already, in Software",
      "Fact checker: use only lowercase letters, digits and dashes",
      "checker: policy.toml has no profile called nope",
    ]);
    expect(within(panel()).getByText("You have a team called Software already. Pick another name.")).toBeTruthy();
    expect(within(panel()).getByText("Pick one agent to coordinate.")).toBeTruthy();
    expect(within(panel()).getByText("Write a prompt: only the coordinator can do without one.")).toBeTruthy();
    expect(within(panel()).getByText("policy.toml has no profile called nope. Pick one it has.")).toBeTruthy();

    // A row with a problem opens; the link focuses its field.
    fireEvent.click(within(summary()).getByRole("button", { name: "checker: policy.toml has no profile called nope" }));
    const profiles = within(panel()).getAllByLabelText("Policy profile");
    expect(document.activeElement).toBe(profiles[profiles.length - 1]);
    expect(document.activeElement!.getAttribute("aria-invalid")).toBe("true");
  });

  test("a closed row counts its problems, and fixing one takes it from the summary", async () => {
    openAt("/agents?team=new&template=writing");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    fireEvent.click(rowButton("writer"));
    fireEvent.change(field("Name"), { target: { value: "reviewer" } });
    fireEvent.click(rowButton("reviewer"));
    await create();

    expect(rowButton("reviewer").getAttribute("aria-label")).toBe("reviewer, 1 problem");
    expect(rowButton("reviewer").getAttribute("aria-expanded")).toBe("true");
    fireEvent.change(field("Name"), { target: { value: "drafter" } });
    expect(document.getElementById("team-summary")).toBeNull();
  });
});

describe("New team: saving", () => {
  test("a profile policy.toml lacks is said before, then offered on the 409, and the retry carries the policy's hash", async () => {
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError("policy.toml lacks read-mostly", 409, { missingProfiles: ["read-mostly"] }))
      .mockResolvedValueOnce({});
    openAt("/agents?team=new&template=visionaries");
    expect(
      within(panel()).getByText("2 agents use read-mostly, which policy.toml doesn't have. Create team offers to add it."),
    ).toBeTruthy();
    expect(rowButton("skeptic").getAttribute("aria-label")).toBe("skeptic, profile missing");
    await create();

    const offer = within(panel()).getByRole("alert");
    expect(offer.textContent).toContain("The Visionaries 2 team needs the read-mostly profile, which your policy file doesn't have");
    expect(offer.textContent).toContain("then creates the Visionaries 2 team.");
    await act(async () => fireEvent.click(within(offer).getByRole("button", { name: "Add the profile and create the team" })));

    expect(sent()[1]).toEqual([
      "POST",
      "/api/teams",
      expect.objectContaining({ team: "Visionaries 2", addMissingProfiles: true, hash: "h1" }),
    ]);
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
  });

  test("the server's refusal shows in the summary and everything typed stays", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError("agent names must be unique", 400));
    openAt("/agents?team=new&template=writing");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await create();

    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't create the team");
    expect(summary().textContent).toContain("agent names must be unique");
    expect(document.activeElement).toBe(summary());
    expect(teamName().value).toBe("Docs");
    expect(rowButton("writer")).toBeTruthy();
    expect(within(panel()).getByRole("button", { name: "Create team" }).hasAttribute("disabled")).toBe(false);
  });

  test("a failed fetch says Agentry couldn't be reached", async () => {
    vi.mocked(api).mockRejectedValueOnce(new TypeError("Failed to fetch"));
    openAt("/agents?team=new");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await create();
    expect(summary().textContent).toContain("Couldn't reach Agentry. Check it's running, then try again.");
  });

  test("while it saves the panel is read-only and can't close", async () => {
    let answer = () => {};
    vi.mocked(api).mockReturnValueOnce(new Promise((resolve) => (answer = () => resolve({}))));
    openAt("/agents?team=new");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await create();

    expect(within(panel()).getByRole("button", { name: "Creating…" }).hasAttribute("disabled")).toBe(true);
    expect(within(panel()).getByRole("button", { name: "Close" }).hasAttribute("disabled")).toBe(true);
    expect(within(panel()).getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(true);
    expect(teamName().readOnly).toBe(true);
    expect(within(panel()).getByRole("button", { name: "Add agent" }).getAttribute("aria-disabled")).toBe("true");
    expect(within(panel()).getByText("Creating the team…")).toBeTruthy();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(panel()).toBeTruthy();
    await act(async () => answer());
  });

  test("the new team shows on the page with its agents as the panel closes", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    afterReload = {
      "/api/agents": [...ROSTER, agent("editor", "Docs", { coordinates: true }), agent("writer", "Docs", { column: "review" })],
    };
    openAt("/agents?team=new&template=writing");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await create();

    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    const heading = screen.getByRole("heading", { name: "Docs" });
    const section = heading.closest("section")!;
    expect(within(section).getByText("editor")).toBeTruthy();
    expect(within(section).getByText("writer")).toBeTruthy();
    expect(screen.getByText("2 teams · 4 agents")).toBeTruthy();
  });
});
