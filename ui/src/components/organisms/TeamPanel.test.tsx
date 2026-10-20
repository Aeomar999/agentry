import { useSyncExternalStore } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { api, ApiError } from "@/api/client";
import type { Agent, Policy, TeamTemplate, TemplateAgent } from "@/api/types";
import { AgentsPage } from "@/pages/AgentsPage";

// The New team panel on the Agents page, with the API answered here: the
// reads from a little store the page's reloads can change, the team's
// POST from a mock.

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn() }));

// The toasts the page shows, as calls.
const { notify } = vi.hoisted(() => ({ notify: vi.fn() }));
vi.mock("@/components/organisms/Toasts", async (actual) => ({
  ...(await actual<typeof import("@/components/organisms/Toasts")>()),
  useToast: () => notify,
}));

// Monaco doesn't run in jsdom: the prompt is a plain box here, which
// keeps the editor's options it was given.
vi.mock("@/components/molecules/CodeViewer", () => ({
  CodeViewer: ({ text, onChange, readOnly, ...options }: { text: string; onChange: (text: string) => void; readOnly?: boolean }) => (
    <textarea value={text} readOnly={readOnly} onChange={(e) => onChange(e.target.value)} data-options={JSON.stringify(options)} />
  ),
}));

const reads: Record<string, unknown> = {};
const errors: Record<string, string> = {}; // why a read failed
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
      if (path in afterReload) {
        reads[path] = afterReload[path];
        delete errors[path];
      }
      changed();
    };
    return { data, error: errors[path] ?? "", position: 1, reload };
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
  notify.mockReset();
  Element.prototype.scrollIntoView = vi.fn();
  Object.assign(reads, {
    "/api/agents": ROSTER,
    "/api/teams/templates": [WRITING, IDEAS],
    "/api/policy": POLICY,
    "/api/config": { engines: ["claude", "codex"] },
  });
  afterReload = {};
  for (const path in errors) delete errors[path];
});

// The router, as the app's, so the panel can hold Back; /tasks is a page
// before the Agents page; an agent's page and a task's stand for the pages
// the panel links to.
let router: ReturnType<typeof createMemoryRouter>;
const openAt = (...history: string[]) => {
  router = createMemoryRouter(
    [
      { path: "/agents", element: <AgentsPage /> },
      { path: "/tasks", element: <p>Tasks</p> },
      { path: "/agents/:name", element: <p>An agent&apos;s page</p> },
      { path: "/tasks/:id", element: <p>A task&apos;s page</p> },
    ],
    { initialEntries: history, initialIndex: history.length - 1 },
  );
  return render(<RouterProvider router={router} />);
};
const address = () => router.state.location.pathname + router.state.location.search;
const back = () => act(async () => router.navigate(-1));

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
              color: "blue",
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

  test("the page is no longer inert when focus lands back on Add a team", () => {
    const { container } = openAt("/agents");
    fireEvent.click(addTeam());
    fireEvent.click(screen.getByRole("button", { name: "Build a team" }));
    let inertAtFocus: boolean | undefined;
    addTeam().addEventListener("focus", () => (inertAtFocus = container.inert));
    fireEvent.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(document.activeElement).toBe(addTeam());
    expect(inertAtFocus).toBe(false);
  });

  test("Cancel puts focus on Add a team, from a link too", () => {
    openAt("/agents?team=new");
    fireEvent.click(within(panel()).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(document.activeElement).toBe(addTeam());
  });

  test("creating the team puts focus on its heading on the page", async () => {
    vi.mocked(api).mockResolvedValue({});
    afterReload = { "/api/agents": [...ROSTER, agent("editor", "Writing 2", { coordinates: true })] };
    openAt("/agents");
    fireEvent.click(addTeam());
    fireEvent.click(screen.getByRole("button", { name: "Change it first, Writing" }));
    await create();
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Writing 2" }));
    expect(document.activeElement!.getAttribute("tabindex")).toBe("-1");
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

  test("what was to be said next frame isn't, once the panel is gone", () => {
    const cancelled = vi.spyOn(window, "cancelAnimationFrame");
    const { unmount } = openAt("/agents?team=new");
    fireEvent.click(within(panel()).getByRole("button", { name: "Add agent" }));
    const scheduled = vi.spyOn(window, "requestAnimationFrame");
    fireEvent.click(within(panel()).getAllByRole("button", { name: "Remove New agent" })[0]);
    const frame = scheduled.mock.results[0].value;
    unmount();
    expect(cancelled).toHaveBeenCalledWith(frame);
    scheduled.mockRestore();
    cancelled.mockRestore();
  });

  test("the prompt editor is 8 to 24 lines, and lets Tab move on and the wheel scroll the panel", () => {
    openAt("/agents?team=new&template=writing");
    const prompt = within(panel()).getByRole("group", { name: "Prompt" }).querySelector("textarea")!;
    expect(JSON.parse(prompt.dataset.options!)).toEqual({
      language: "markdown",
      minHeight: 180,
      maxHeight: 500,
      tabFocusMode: true,
      alwaysConsumeMouseWheel: false,
      placeholder: "Empty: it uses Agentry's coordinator prompt for a team of these agents.",
    });
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

describe("New team: history and unsaved changes", () => {
  const buildFromThePage = () => {
    fireEvent.click(addTeam());
    fireEvent.click(screen.getByRole("button", { name: "Build a team" }));
  };
  const question = () => within(panel()).queryByRole("group", { name: "Discard this new team?" });
  const closeButton = () => within(panel()).getByRole("button", { name: "Close" });
  const cancel = () => within(panel()).getByRole("button", { name: "Cancel" });

  test("opened from the page, it has an entry of its own, which Close goes back over", async () => {
    openAt("/tasks", "/agents");
    buildFromThePage();
    expect(address()).toBe("/agents?team=new");
    fireEvent.click(closeButton());
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("Back with no changes closes it", async () => {
    openAt("/tasks", "/agents");
    buildFromThePage();
    await back();
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("Back with changes keeps it open and asks in the footer; Discard then closes", async () => {
    openAt("/tasks", "/agents");
    buildFromThePage();
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await back();
    expect(address()).toBe("/agents?team=new");
    expect(question()!.textContent).toContain("It hasn't been created yet.");
    expect(document.activeElement).toBe(within(question()!).getByRole("button", { name: "Keep editing" }));
    fireEvent.click(within(question()!).getByRole("button", { name: "Keep editing" }));
    expect(question()).toBeNull();
    expect(teamName().value).toBe("Docs");

    await back();
    await act(async () => fireEvent.click(within(question()!).getByRole("button", { name: "Discard" })));
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("Back pressed while it saves is let go: a refusal then shows, with no question", async () => {
    let refuse = () => {};
    vi.mocked(api).mockReturnValueOnce(
      new Promise((_, reject) => (refuse = () => reject(new ApiError("agent names must be unique", 400)))),
    );
    openAt("/tasks", "/agents");
    buildFromThePage();
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    await create();
    await back();
    await act(async () => refuse());

    expect(summary().textContent).toContain("agent names must be unique");
    expect(within(panel()).queryByText(/Discard this new team/)).toBeNull();
    expect(address()).toBe("/agents?team=new");
  });

  test("two Cancels before the first lands go back once", async () => {
    openAt("/tasks", "/agents");
    buildFromThePage();
    await act(async () => {
      fireEvent.click(cancel());
      fireEvent.click(cancel());
    });
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("Back while it asks still puts focus back where it was on Keep editing", async () => {
    openAt("/tasks", "/agents");
    buildFromThePage();
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    teamName().focus();
    fireEvent.click(closeButton());
    await back();
    fireEvent.click(within(question()!).getByRole("button", { name: "Keep editing" }));
    expect(document.activeElement).toBe(teamName());
  });

  test("opened from a link, closing takes the panel off the address instead", async () => {
    openAt("/tasks", "/agents?view=org&team=new&template=writing");
    fireEvent.click(cancel());
    expect(address()).toBe("/agents?view=org");
    expect(router.state.historyAction).toBe("REPLACE");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("Esc, Close and Cancel with changes ask first; Esc again or Keep editing puts focus back", () => {
    openAt("/agents?team=new");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    teamName().focus();

    fireEvent.keyDown(window, { key: "Escape" });
    expect(document.activeElement).toBe(within(question()!).getByRole("button", { name: "Keep editing" }));
    expect(within(panel()).queryByRole("button", { name: "Create team" })).toBeNull();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(question()).toBeNull();
    expect(document.activeElement).toBe(teamName());

    closeButton().focus();
    fireEvent.click(closeButton());
    fireEvent.click(within(question()!).getByRole("button", { name: "Keep editing" }));
    expect(document.activeElement).toBe(closeButton());

    cancel().focus();
    fireEvent.click(cancel());
    fireEvent.click(within(question()!).getByRole("button", { name: "Keep editing" }));
    expect(document.activeElement).toBe(cancel());

    fireEvent.click(cancel());
    fireEvent.click(within(question()!).getByRole("button", { name: "Discard" }));
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("a change typed back is no change: Close closes at once", () => {
    openAt("/agents?team=new&template=writing");
    fireEvent.click(rowButton("writer"));
    fireEvent.change(field("Description"), { target: { value: "Drafts." } });
    fireEvent.change(field("Description"), { target: { value: "The writer." } });
    fireEvent.click(closeButton());
    expect(screen.queryByRole("dialog", { name: "New team" })).toBeNull();
  });

  test("reloading or closing the tab asks the browser's question only with changes", () => {
    openAt("/agents?team=new");
    const leave = () => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    };
    expect(leave()).toBe(false);
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    expect(leave()).toBe(true);
  });
});

describe("New team: opened cold", () => {
  test("Team name and Start from work while it loads; Create team waits", () => {
    delete reads["/api/teams/templates"];
    delete reads["/api/config"];
    openAt("/agents?team=new");
    fireEvent.change(teamName(), { target: { value: "Docs" } });
    fireEvent.click(within(panel()).getByRole("radio", { name: "Copy a template" }));

    const template = within(panel()).getByLabelText("Template") as HTMLSelectElement;
    expect(template.disabled).toBe(true);
    expect(template.textContent).toBe("Loading templates…");
    expect(within(panel()).getByText("Loading engines and profiles…").getAttribute("aria-live")).toBe("polite");
    const createTeam = within(panel()).getByRole("button", { name: "Create team" });
    expect(createTeam.hasAttribute("disabled")).toBe(true);
    expect(document.getElementById(createTeam.getAttribute("aria-describedby")!)!.textContent).toBe("Loading…");

    act(() => {
      reads["/api/teams/templates"] = [WRITING, IDEAS];
      reads["/api/config"] = { engines: ["claude", "codex"] };
      changed();
    });
    expect((within(panel()).getByLabelText("Template") as HTMLSelectElement).value).toBe("writing");
    expect(rowButton("writer")).toBeTruthy();
    expect(teamName().value).toBe("Docs");
    expect(within(panel()).getByRole("button", { name: "Create team" }).hasAttribute("disabled")).toBe(false);
  });

  test("a failed load says so in the server's words, with Try again, and only Cancel", async () => {
    delete reads["/api/config"];
    errors["/api/config"] = "the config is unreadable";
    openAt("/agents?team=new");

    const alert = within(panel()).getByRole("alert");
    expect(alert.textContent).toContain("Couldn't load what a new team needs");
    expect(alert.textContent).toContain("the config is unreadable");
    expect(document.activeElement).toBe(within(alert).getByRole("button", { name: "Try again" }));
    expect(within(panel()).queryByRole("button", { name: "Create team" })).toBeNull();
    expect(within(panel()).getByRole("button", { name: "Cancel" })).toBeTruthy();

    afterReload = { "/api/config": { engines: ["claude"] } };
    await act(async () => fireEvent.click(within(alert).getByRole("button", { name: "Try again" })));
    expect(within(panel()).queryByRole("alert")).toBeNull();
    expect(rowButton("coordinator")).toBeTruthy();
  });

  test("a load that couldn't reach Agentry says so in our words", () => {
    delete reads["/api/teams/templates"];
    errors["/api/teams/templates"] = new TypeError("Failed to fetch").message;
    openAt("/agents?team=new");

    const alert = within(panel()).getByRole("alert");
    expect(alert.textContent).toContain("Couldn't reach Agentry. Check it's running, then try again.");
    expect(alert.textContent).not.toContain("Failed to fetch");
  });
});

// The Docs team, as the Edit team panel opens it: editor coordinates writer and proofer.
const DOCS = [
  agent("editor", "Docs", { coordinates: true, description: "Leads the docs.", prompt: "You lead.", color: "violet", teamVersion: "v1" }),
  agent("writer", "Docs", { description: "Writes.", prompt: "You write.", color: "blue", teamVersion: "v1" }),
  agent("proofer", "Docs", { prompt: "You proof.", column: "review", teamVersion: "v1" }),
];

// What saving the Docs team sends for a row: its name then (was), and now.
const sentRow = (was: string, given: Partial<Agent> = {}) => {
  const { name, description, coordinates, prompt, engine, model, profile, color, column } = {
    ...DOCS.find((a) => a.name === was)!,
    ...given,
  };
  return { was, name, description, coordinates, prompt, engine, model, profile, color, column };
};

const sheet = () => screen.getByRole("dialog");
const inSheet = () => within(sheet());
const docsRow = (name: string) => inSheet().getByRole("button", { name: new RegExp(`^${name}(,|$)`) });
const saveButton = () => inSheet().getByRole("button", { name: /^(Save changes|Saving…)$/ });
const save = () => act(async () => fireEvent.click(saveButton()));
// settle lets a refusal's reload of the roster land.
const settle = () => act(async () => new Promise((resolve) => setTimeout(resolve, 0)));
const typeInto = (label: string, value: string) => fireEvent.change(inSheet().getByLabelText(label), { target: { value } });

describe("Edit team: opening", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });

  test("Edit team on a heading opens the team, its agents closed, focus on the title", () => {
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));

    expect(address()).toBe("/agents?team=Docs");
    expect(sheet().getAttribute("aria-labelledby")).toBeTruthy();
    expect(document.activeElement).toBe(inSheet().getByRole("heading", { name: "Edit the Docs team" }));
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).value).toBe("Docs");
    expect(["editor", "writer", "proofer"].map((name) => docsRow(name).getAttribute("aria-expanded"))).toEqual(["false", "false", "false"]);
    expect(inSheet().queryByText("Start from")).toBeNull();
    expect(saveButton().hasAttribute("disabled")).toBe(true);
    expect(saveButton().getAttribute("aria-describedby")).toBe("team-footer-text");
    expect(document.getElementById("team-footer-text")!.textContent).toBe("No changes to save.");
  });

  test("every heading has Edit team first, then + Add agent and Remove team, each named with its team", () => {
    openAt("/agents");
    const heading = screen.getByRole("heading", { name: "Docs" }).parentElement!;
    expect(
      within(heading)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Edit team, Docs", "Add agent to Docs", "Remove team, Docs"]);
  });

  test("+ Add agent opens the team with a new agent open and focus in its Name", () => {
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Add agent to Docs" }));

    expect(address()).toBe("/agents?team=Docs&add=agent");
    expect(docsRow("New agent").getAttribute("aria-label")).toBe("New agent, new");
    expect(docsRow("New agent").getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(inSheet().getByLabelText("Name"));
    expect(inSheet().getByText("Knowledge and where it runs: on its own page, once you save.")).toBeTruthy();
    expect(saveButton().hasAttribute("disabled")).toBe(false);
  });

  test("an open row links to its agent's page by the name it has saved", () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    typeInto("Name", "drafter");
    expect(inSheet().getByRole("link", { name: "Open writer's page" }).getAttribute("href")).toBe("/agents/writer");
    expect(inSheet().getByText(/^Renames writer: it keeps its prompt/)).toBeTruthy();
  });

  test("opened cold it says it is loading, with no fields and Save changes off, then shows the team", () => {
    delete reads["/api/agents"];
    openAt("/agents?team=Docs");

    expect(inSheet().getByRole("heading", { name: "Edit the Docs team" })).toBeTruthy();
    expect(inSheet().getByText("Loading the Docs team…")).toBeTruthy();
    expect(inSheet().queryByLabelText("Team name")).toBeNull();
    expect(saveButton().hasAttribute("disabled")).toBe(true);
    expect(document.getElementById("team-footer-text")!.textContent).toBe("Loading…");

    act(() => {
      reads["/api/agents"] = [...ROSTER, ...DOCS];
      changed();
    });
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).value).toBe("Docs");
    expect(docsRow("writer")).toBeTruthy();
  });

  test("a failed load says so, with Try again and only Cancel", async () => {
    delete reads["/api/agents"];
    errors["/api/agents"] = "the log is unreadable";
    afterReload = { "/api/agents": [...ROSTER, ...DOCS] };
    openAt("/agents?team=Docs");

    const alert = inSheet().getByRole("alert");
    expect(alert.textContent).toContain("Couldn't load the team");
    expect(alert.textContent).toContain("the log is unreadable");
    expect(inSheet().queryByRole("button", { name: "Save changes" })).toBeNull();
    expect(inSheet().getByRole("button", { name: "Cancel" })).toBeTruthy();
    await act(async () => fireEvent.click(within(alert).getByRole("button", { name: "Try again" })));
    expect(docsRow("editor")).toBeTruthy();
  });

  test("no team by that name: the title says so and takes focus, with no fields, and goes back to the Agents page", () => {
    openAt("/agents?team=Handbook");

    const title = inSheet().getByRole("heading", { name: "Couldn't find the Handbook team" });
    expect(document.activeElement).toBe(title);
    expect(inSheet().getByText(/^It may have been renamed or removed since this link was made/)).toBeTruthy();
    expect(inSheet().queryByLabelText("Team name")).toBeNull();
    expect(inSheet().queryByRole("button", { name: "Cancel" })).toBeNull();
    fireEvent.click(inSheet().getByRole("button", { name: "Back to the Agents page" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("a team with two coordinators opens with no radio checked and the error under the legend", () => {
    reads["/api/agents"] = [...ROSTER, ...DOCS.map((a) => (a.name === "writer" ? { ...a, coordinates: true } : a))];
    openAt("/agents?team=Docs");

    const radios = inSheet().getAllByRole("radio") as HTMLInputElement[];
    expect(radios.some((radio) => radio.checked)).toBe(false);
    expect(docsRow("editor").textContent).toContain("coordinator");
    expect(docsRow("writer").textContent).toContain("coordinator");
    expect(inSheet().getByText("editor and writer both coordinate. Pick the one that takes the team's tasks.")).toBeTruthy();
    // Said under the legend at once; the summary and the footer count wait for a save.
    expect(document.getElementById("team-summary")).toBeNull();
    expect(document.getElementById("team-footer-text")!.textContent).toBe("No changes to save.");
    fireEvent.click(inSheet().getByRole("radio", { name: "Coordinator, writer" }));
    expect(inSheet().queryByText(/both coordinate/)).toBeNull();
  });
});

describe("Edit team: changing it", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });

  test("renaming the team says what follows, and Save changes sends the team in one PUT", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Handbook", version: "v2" });
    afterReload = { "/api/agents": [...ROSTER, ...DOCS.map((a) => ({ ...a, team: "Handbook" }))] };
    openAt("/agents?team=Docs");
    typeInto("Team name", " Handbook ");

    expect(inSheet().getByText("Renames Docs: its agents and their tasks move with it.")).toBeTruthy();
    expect(document.getElementById("team-footer-text")!.textContent).toBe("");
    await save();

    expect(sent()).toEqual([
      ["PUT", "/api/teams/Docs", { version: "v1", team: "Handbook", agents: [sentRow("editor"), sentRow("writer"), sentRow("proofer")] }],
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Handbook" }));
  });

  test("renamed only in case from its Edit team button, the heading takes focus: the button has gone", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "docs", version: "v2" });
    afterReload = { "/api/agents": [...ROSTER, ...DOCS.map((a) => ({ ...a, team: "docs" }))] };
    openAt("/agents");
    const edit = screen.getByRole("button", { name: "Edit team, Docs" });
    edit.focus();
    fireEvent.click(edit);
    typeInto("Team name", "docs");
    await save();

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(edit.isConnected).toBe(false);
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "docs" }));
  });

  test("saved unrenamed from its Edit team button, focus goes back to the button", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", version: "v2" });
    openAt("/agents");
    const edit = screen.getByRole("button", { name: "Edit team, Docs" });
    edit.focus();
    fireEvent.click(edit);
    fireEvent.click(docsRow("writer"));
    typeInto("Description", "Drafts.");
    await save();

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Edit team, Docs" }));
  });

  test("saved unrenamed from a link that spells the team exactly, the heading takes focus: there is no button to go back to", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", version: "v2" });
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    typeInto("Description", "Drafts.");
    await save();

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Docs" }));
  });

  test("a renamed, changed, added and removed agent each say so until saved, and all go in the PUT", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", version: "v2" });
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    typeInto("Name", "drafter");
    fireEvent.click(docsRow("editor"));
    typeInto("Description", "Leads.");
    fireEvent.click(inSheet().getByRole("button", { name: "Remove proofer" }));
    fireEvent.click(inSheet().getByRole("button", { name: "Add agent" }));
    typeInto("Name", "critic");
    fireEvent.change(inSheet().getByRole("group", { name: "Prompt" }).querySelector("textarea")!, { target: { value: "You critique." } });

    expect(docsRow("drafter").getAttribute("aria-label")).toBe("drafter, edited, was writer");
    expect(docsRow("editor").getAttribute("aria-label")).toBe("editor, edited");
    expect(docsRow("critic").getAttribute("aria-label")).toBe("critic, new");
    expect(inSheet().getByText("proofer will be removed when you save")).toBeTruthy();
    expect(inSheet().getByText("3 agents · 4 changed")).toBeTruthy();
    await save();

    const critic = { name: "critic", description: "", coordinates: false, prompt: "You critique.", engine: "claude", model: "" };
    expect(sent()).toEqual([
      [
        "PUT",
        "/api/teams/Docs",
        {
          version: "v1",
          team: "Docs",
          agents: [
            sentRow("editor", { description: "Leads." }),
            sentRow("writer", { name: "drafter" }),
            { was: "", ...critic, profile: "", color: "teal", column: "" },
          ],
        },
      ],
    ]);
  });

  test("in Edit the prompt says renaming doesn't change it, with the old name once renamed", () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));

    expect(inSheet().getByText("Renaming an agent doesn't change its prompt.")).toBeTruthy();
    typeInto("Name", "drafter");
    expect(inSheet().queryByText("Renaming an agent doesn't change its prompt.")).toBeNull();
    expect(inSheet().getByText(`Renaming doesn't change its prompt: it may still say "writer".`)).toBeTruthy();
  });

  test("a new agent made coordinator in Edit still offers Agentry's coordinator prompt, with no renaming note", () => {
    openAt("/agents?team=Docs&add=agent");
    fireEvent.click(inSheet().getByRole("radio", { name: "Coordinator, New agent" }));

    expect(inSheet().getByText("Empty: it uses Agentry's coordinator prompt for a team of these agents.")).toBeTruthy();
    const prompt = inSheet().getByRole("group", { name: "Prompt" }).querySelector("textarea")!;
    expect(JSON.parse(prompt.dataset.options!).placeholder).toBe("Empty: it uses Agentry's coordinator prompt for a team of these agents.");
    expect(inSheet().queryByText(/^Renaming/)).toBeNull();
  });

  test("the prompt's line count says changed once it differs from the saved prompt", () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    const prompt = inSheet().getByRole("group", { name: "Prompt" }).querySelector("textarea")!;

    expect(inSheet().getByText("1 line")).toBeTruthy();
    typeInto("Description", "Drafts.");
    expect(inSheet().getByText("1 line")).toBeTruthy();
    fireEvent.change(prompt, { target: { value: "You write.\nPlainly." } });
    expect(inSheet().getByText("2 lines · changed")).toBeTruthy();
    fireEvent.change(prompt, { target: { value: "You write." } });
    expect(inSheet().getByText("1 line")).toBeTruthy();
  });

  test("removing an agent leaves a stub with Undo, which takes focus; Undo puts it back", () => {
    openAt("/agents?team=Docs");
    fireEvent.click(inSheet().getByRole("button", { name: "Remove proofer" }));

    expect(document.activeElement).toBe(inSheet().getByRole("button", { name: "Undo, keep proofer" }));
    fireEvent.click(inSheet().getByRole("button", { name: "Undo, keep proofer" }));
    expect(document.activeElement).toBe(docsRow("proofer"));
    expect(saveButton().hasAttribute("disabled")).toBe(true);
  });

  test("the coordinator it has can't lose its prompt: only a new one goes without", async () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("editor"));
    const prompt = inSheet().getByRole("group", { name: "Prompt" }).querySelector("textarea")!;
    fireEvent.change(prompt, { target: { value: "" } });

    // Neither the hint nor the empty box offers Agentry's coordinator prompt.
    expect(inSheet().queryByText(/^Empty: it uses Agentry's coordinator prompt/)).toBeNull();
    expect(JSON.parse(prompt.dataset.options!).placeholder).toBeUndefined();
    await save();

    expect(sent()).toEqual([]);
    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't save the team: 1 thing to fix");
    expect(inSheet().getByText("Write a prompt: Agentry's coordinator prompt is only for a new agent.")).toBeTruthy();
  });

  test("Cancel with changes asks to discard them, counting them; Keep editing stays, Discard closes", () => {
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    fireEvent.click(docsRow("writer"));
    typeInto("Name", "drafter");
    fireEvent.click(inSheet().getByRole("button", { name: "Remove proofer" }));
    fireEvent.click(inSheet().getByRole("button", { name: "Add agent" }));
    fireEvent.click(inSheet().getByRole("button", { name: "Cancel" }));

    const question = inSheet().getByRole("group", { name: "Discard your changes to Docs?" });
    expect(question.textContent).toContain("The team renamed, 1 agent changed, 1 added and 1 removed. They aren't saved.");
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep editing" }));
    fireEvent.click(within(question).getByRole("button", { name: "Keep editing" }));
    expect(sheet()).toBeTruthy();
    fireEvent.click(inSheet().getByRole("button", { name: "Cancel" }));
    fireEvent.click(inSheet().getByRole("button", { name: "Discard" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("with no changes Close closes at once", () => {
    openAt("/agents?team=Docs");
    fireEvent.click(inSheet().getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("Edit team: saving", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });

  test("while it saves the panel is read-only and can't close", async () => {
    let answer = () => {};
    vi.mocked(api).mockReturnValueOnce(new Promise((resolve) => (answer = () => resolve({ team: "Handbook", version: "v2" }))));
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();

    expect(saveButton().textContent).toBe("Saving…");
    expect(saveButton().hasAttribute("disabled")).toBe(true);
    expect(inSheet().getByRole("button", { name: "Close" }).hasAttribute("disabled")).toBe(true);
    expect(inSheet().getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(true);
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).readOnly).toBe(true);
    expect(inSheet().getByRole("button", { name: "Remove writer" }).getAttribute("aria-disabled")).toBe("true");
    expect(inSheet().getByText("Saving the team…")).toBeTruthy();
    await act(async () => answer());
  });

  test("problems the server finds go on their rows, and the summary links to them", async () => {
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError("the Docs team can't be saved", 400, { problems: [{ row: 1, field: "name", error: "writer is a reserved name" }] }),
    );
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();

    expect(document.activeElement).toBe(summary());
    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't save the team: 1 thing to fix");
    expect(docsRow("writer").getAttribute("aria-expanded")).toBe("true");
    expect(inSheet().getByText("writer is a reserved name.")).toBeTruthy();
    fireEvent.click(within(summary()).getByRole("button", { name: "writer: writer is a reserved name" }));
    expect(document.activeElement).toBe(inSheet().getByLabelText("Name"));
  });

  test("a removed agent tasks still need says so on its stub; its link goes to Undo, which clears it", async () => {
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError("the Docs team can't be saved", 409, {
        problems: [{ field: "removed", agent: "proofer", error: "proofer is still needed by task-1: finish those first, or keep proofer" }],
      }),
    );
    openAt("/agents?team=Docs");
    fireEvent.click(inSheet().getByRole("button", { name: "Remove proofer" }));
    await save();

    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't save the team: 1 thing to fix");
    const stub = inSheet().getByText("proofer will be removed when you save").closest("li")!;
    expect(stub.textContent).toContain("Still needed by task-1. Finish it first, or undo to keep proofer.");
    expect(within(stub).getByRole("link", { name: "task-1" }).getAttribute("href")).toBe("/tasks/task-1");
    fireEvent.click(within(summary()).getByRole("button", { name: "proofer: still needed by task-1" }));
    expect(document.activeElement).toBe(within(stub).getByRole("button", { name: "Undo, keep proofer" }));
    fireEvent.click(within(stub).getByRole("button", { name: "Undo, keep proofer" }));
    expect(document.getElementById("team-summary")).toBeNull();
  });

  test("changed elsewhere: keep my changes closes the box, focus on the title, and saving is refused again", async () => {
    const changedElsewhere = new ApiError("changed elsewhere", 409, { error: "changed elsewhere", version: "v2" });
    vi.mocked(api).mockRejectedValueOnce(changedElsewhere).mockRejectedValueOnce(changedElsewhere);
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();

    expect(document.activeElement).toBe(summary());
    expect(summary().textContent).toContain("Couldn't save the team: it changed since you opened it");
    expect(summary().textContent).toContain("The Docs team was changed somewhere else after you opened it.");
    fireEvent.click(within(summary()).getByRole("button", { name: "Keep my changes here" }));
    expect(document.getElementById("team-summary")).toBeNull();
    expect(document.activeElement).toBe(inSheet().getByRole("heading", { name: "Edit the Docs team" }));
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).value).toBe("Handbook");
    expect(saveButton().hasAttribute("disabled")).toBe(false);
    await save();

    // Still the version it loaded: saving over the team would delete agents added elsewhere.
    expect(sent()[1]).toEqual(["PUT", "/api/teams/Docs", expect.objectContaining({ version: "v1", team: "Handbook" })]);
    expect(summary().textContent).toContain("Couldn't save the team: it changed since you opened it");
  });

  test("changed elsewhere: reloading asks first, then shows the team as it is now", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError("changed elsewhere", 409, { error: "changed elsewhere", version: "v2" }));
    afterReload = {
      "/api/agents": [
        ...ROSTER,
        ...DOCS.map((a) => ({ ...a, teamVersion: "v2" })),
        agent("indexer", "Docs", { prompt: "You index.", teamVersion: "v2" }),
      ],
    };
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();
    fireEvent.click(within(summary()).getByRole("button", { name: "Reload the team" }));

    const question = within(summary()).getByRole("group", { name: "Reload and lose your changes?" });
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep editing" }));
    await act(async () => fireEvent.click(within(question).getByRole("button", { name: "Reload" })));
    await settle();

    expect(document.getElementById("team-summary")).toBeNull();
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).value).toBe("Docs");
    expect(document.activeElement).toBe(inSheet().getByLabelText("Team name"));
    expect(docsRow("indexer")).toBeTruthy();
    expect(saveButton().hasAttribute("disabled")).toBe(true);
  });

  test("cut short: the team is saved as reloaded, and Save again saves it unchanged under its new names", async () => {
    const error = "saved the Handbook team, but 1 task still needs moving: save again to finish (task-1: busy)";
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError(error, 409, { error, version: "v3" }))
      .mockResolvedValueOnce({ team: "Handbook", version: "v4" });
    afterReload = {
      "/api/agents": [
        ...ROSTER,
        ...DOCS.map((a) => ({ ...a, team: "Handbook", teamVersion: "v3", name: a.name === "writer" ? "drafter" : a.name })),
      ],
    };
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    fireEvent.click(docsRow("writer"));
    typeInto("Name", "drafter");
    await save();
    await settle();

    expect(summary().textContent).toContain("Saved the Handbook team, but some of its tasks haven't moved yet");
    expect(summary().textContent).toContain(`Save again to finish moving them. Agentry said: "${error}"`);
    expect(inSheet().getByRole("heading", { name: "Edit the Handbook team" })).toBeTruthy();
    expect(docsRow("drafter").getAttribute("aria-label")).toBe("drafter");
    await act(async () => fireEvent.click(within(summary()).getByRole("button", { name: "Save again" })));

    expect(sent()[1]).toEqual([
      "PUT",
      "/api/teams/Handbook",
      {
        version: "v3",
        team: "Handbook",
        agents: [sentRow("editor"), { ...sentRow("writer", { name: "drafter" }), was: "drafter" }, sentRow("proofer")],
      },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("Back while Save again runs, with nothing left unsaved, stays put", async () => {
    const error = "saved the Handbook team, but 1 task still needs moving: save again to finish (task-1: busy)";
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError(error, 409, { error, version: "v3" }))
      .mockReturnValueOnce(new Promise(() => {}));
    afterReload = { "/api/agents": [...ROSTER, ...DOCS.map((a) => ({ ...a, team: "Handbook", teamVersion: "v3" }))] };
    openAt("/tasks", "/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    typeInto("Team name", "Handbook");
    await save();
    await settle();
    await act(async () => fireEvent.click(within(summary()).getByRole("button", { name: "Save again" })));
    await back();

    expect(address()).toBe("/agents?team=Docs");
    expect(saveButton().textContent).toBe("Saving…");
  });

  test("a profile policy.toml lacks is offered on the 409, and the retry carries the policy's hash", async () => {
    reads["/api/agents"] = [...ROSTER, ...DOCS.map((a) => (a.name === "writer" ? { ...a, profile: "read-mostly" } : a))];
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError("policy.toml lacks read-mostly", 409, { missingProfiles: ["read-mostly"] }))
      .mockResolvedValueOnce({ team: "Docs", version: "v2" });
    openAt("/agents?team=Docs");
    expect(inSheet().getByText("1 agent uses read-mostly, which policy.toml doesn't have. Save changes offers to add it.")).toBeTruthy();
    fireEvent.click(docsRow("editor"));
    typeInto("Description", "Leads.");
    await save();

    const offer = inSheet().getByRole("alert");
    expect(offer.textContent).toContain("then saves your changes.");
    await act(async () => fireEvent.click(within(offer).getByRole("button", { name: "Add the profile and save the team" })));
    expect(sent()[1]).toEqual(["PUT", "/api/teams/Docs", expect.objectContaining({ addMissingProfiles: true, hash: "h1" })]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("any other refusal shows in the summary in the server's words, and everything typed stays", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError("the log is unwritable", 500));
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();

    expect(within(summary()).getByRole("heading").textContent).toBe("Couldn't save the team");
    expect(summary().textContent).toContain("the log is unwritable");
    expect((inSheet().getByLabelText("Team name") as HTMLInputElement).value).toBe("Handbook");
    expect(saveButton().hasAttribute("disabled")).toBe(false);
  });
});

// Remove team, in the footer on desktop and under This team on a phone:
// both are drawn, and CSS shows one.
const removeButton = (place: "footer" | "body" = "footer") => document.getElementById(`team-remove-${place}-button`) as HTMLButtonElement;
const removeQuestion = (place: "footer" | "body" = "footer") =>
  document.getElementById(`team-remove-${place}-question`)?.closest<HTMLElement>("[role=group]") ?? null;
const STILL_NEEDED =
  "the Docs team is still needed by task-182, task-190: finish those first (dragging a task to Done on the board finishes it as it is)";

describe("Edit team: Remove team", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });

  test("Remove team asks in the footer, focus on Keep; Keep puts the footer back and focus on Remove team", () => {
    openAt("/agents?team=Docs");
    expect(removeButton().textContent).toBe("Remove team");
    fireEvent.click(removeButton());

    const question = removeQuestion()!;
    expect(question.getAttribute("aria-labelledby")).toBe("team-remove-footer-question");
    expect(question.textContent).toContain("Remove the Docs team and its 3 agents?");
    expect(question.textContent).toContain("Their prompts and what they learned go too.");
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep" }));
    expect(sent()).toEqual([]);

    fireEvent.click(within(question).getByRole("button", { name: "Keep" }));
    expect(removeQuestion()).toBeNull();
    expect(document.activeElement).toBe(removeButton());
  });

  test("with unsaved changes the question says they go too; Esc is Keep", () => {
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    fireEvent.click(removeButton());

    expect(removeQuestion()!.textContent).toContain("Their prompts, what they learned and your unsaved changes go too.");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(removeQuestion()).toBeNull();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  test("Remove removes the team, closes the panel going back over its entry, says so and shows the page without it", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", removed: ["proofer", "writer", "editor"] });
    afterReload = { "/api/agents": ROSTER };
    openAt("/tasks", "/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    expect(sent()).toEqual([["DELETE", "/api/teams/Docs", undefined]]);
    expect(notify).toHaveBeenCalledWith("success", "Removed the Docs team");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Docs" })).toBeNull();
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("removed, focus goes to the next team's heading: its Edit team button went with it", async () => {
    reads["/api/agents"] = [...DOCS, ...ROSTER];
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", removed: ["proofer", "writer", "editor"] });
    afterReload = { "/api/agents": ROSTER };
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    expect(document.activeElement).toBe(screen.getByRole("heading", { level: 2 }));
  });

  test("removed with no team after it, focus goes to the page's heading", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", removed: ["proofer", "writer", "editor"] });
    afterReload = { "/api/agents": ROSTER };
    openAt("/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1, name: "Agents" }));
  });

  test("with unsaved changes, removing closes without asking about them again", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", removed: ["proofer", "writer", "editor"] });
    afterReload = { "/api/agents": ROSTER };
    openAt("/tasks", "/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    typeInto("Team name", "Handbook");
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("while it removes, the question says so and nothing else can be pressed", async () => {
    vi.mocked(api).mockReturnValueOnce(new Promise(() => {}));
    openAt("/agents?team=Docs");
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    const question = removeQuestion()!;
    expect(within(question).getByRole("button", { name: "Removing…" }).hasAttribute("disabled")).toBe(true);
    expect(within(question).getByRole("button", { name: "Keep" }).hasAttribute("disabled")).toBe(true);
    expect(inSheet().getByRole("button", { name: "Close" }).hasAttribute("disabled")).toBe(true);
    expect(inSheet().getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(true);
  });

  test.each([
    ["no changes", false],
    ["unsaved changes", true],
  ])("Back while it removes, with %s, stays put; the panel then goes back over its entry once", async (_, typed) => {
    let removed = () => {};
    vi.mocked(api).mockReturnValueOnce(new Promise((resolve) => (removed = () => resolve({ team: "Docs", removed: [] }))));
    afterReload = { "/api/agents": ROSTER };
    openAt("/tasks", "/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    if (typed) typeInto("Team name", "Handbook");
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));
    await back();

    expect(address()).toBe("/agents?team=Docs");
    expect(within(removeQuestion()!).getByRole("button", { name: "Removing…" })).toBeTruthy();
    expect(inSheet().queryByRole("group", { name: "Discard your changes to Docs?" })).toBeNull();
    await act(async () => removed());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("a refusal quotes the server, links its tasks and takes focus; Remove stays off until the roster changes", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError(STILL_NEEDED, 409));
    openAt("/agents?team=Docs");
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    const question = removeQuestion()!;
    const refusal = within(question).getByText(`It wasn't removed: “${STILL_NEEDED}”`).parentElement!;
    expect(document.activeElement).toBe(refusal);
    expect(refusal.textContent).toContain("Tasks: task-182, task-190");
    expect(within(refusal).getByRole("link", { name: "task-182" }).getAttribute("href")).toBe("/tasks/task-182");
    expect(within(refusal).getByRole("link", { name: "task-190" }).getAttribute("href")).toBe("/tasks/task-190");
    const remove = within(question).getByRole("button", { name: "Remove" });
    expect(remove.hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("dialog")).toBeTruthy();

    act(() => {
      reads["/api/agents"] = [...ROSTER, ...DOCS];
      changed();
    });
    expect(remove.hasAttribute("disabled")).toBe(false);
  });

  test("your only team can't be removed, and says why beside the button", () => {
    reads["/api/agents"] = DOCS;
    openAt("/agents?team=Docs");

    const remove = removeButton();
    expect(remove.hasAttribute("disabled")).toBe(true);
    expect(document.getElementById(remove.getAttribute("aria-describedby")!)!.textContent).toBe(
      "Can't remove your only team: tasks need its coordinator.",
    );
  });

  test("on a phone, Remove team is under This team after + Add agent, and asks there; the footer keeps Cancel and Save changes", () => {
    openAt("/agents?team=Docs");
    const section = inSheet().getByRole("region", { name: "This team" });
    expect(section.className).toContain("sm:hidden");
    expect(section.contains(removeButton("body"))).toBe(true);
    const addAgent = inSheet().getByRole("button", { name: "Add agent" });
    expect(addAgent.compareDocumentPosition(section) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // The footer's Remove team is gone under 640px.
    expect(removeButton().closest(".max-sm\\:hidden")).toBeTruthy();

    fireEvent.click(removeButton("body"));
    expect(document.activeElement).toBe(within(removeQuestion("body")!).getByRole("button", { name: "Keep" }));
    // The footer's question is desktop's; on a phone the footer stays as it is.
    expect(removeQuestion()!.closest(".max-sm\\:hidden")).toBeTruthy();
    expect(saveButton().closest(".sm\\:hidden")).toBeTruthy();
    fireEvent.click(within(removeQuestion("body")!).getByRole("button", { name: "Keep" }));
    expect(document.activeElement).toBe(removeButton("body"));
  });

  test("on a phone, while Remove team asks, Cancel and Save changes are off; Keep turns them on again", () => {
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    const cancel = inSheet().getByRole("button", { name: "Cancel" });
    fireEvent.click(removeButton("body"));
    expect(cancel.hasAttribute("disabled")).toBe(true);
    expect(saveButton().hasAttribute("disabled")).toBe(true);

    fireEvent.click(within(removeQuestion("body")!).getByRole("button", { name: "Keep" }));
    expect(cancel.hasAttribute("disabled")).toBe(false);
    expect(saveButton().hasAttribute("disabled")).toBe(false);
  });

  test.each([
    ["asks", false],
    ["removes", true],
  ])("while Remove team %s, the profile offer's add-and-save is off", async (_, removing) => {
    reads["/api/agents"] = [...ROSTER, ...DOCS.map((a) => (a.name === "writer" ? { ...a, profile: "read-mostly" } : a))];
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError("policy.toml lacks read-mostly", 409, { missingProfiles: ["read-mostly"] }))
      .mockReturnValueOnce(new Promise(() => {}));
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();
    const add = within(inSheet().getByRole("alert")).getByRole("button", { name: "Add the profile and save the team" });
    fireEvent.click(removeButton("body"));
    if (removing) await act(async () => fireEvent.click(within(removeQuestion("body")!).getByRole("button", { name: "Remove" })));

    expect(add.hasAttribute("disabled")).toBe(true);
    if (removing) return;
    fireEvent.click(within(removeQuestion("body")!).getByRole("button", { name: "Keep" }));
    expect(add.hasAttribute("disabled")).toBe(false);
  });

  test.each([
    ["asks", false],
    ["removes", true],
  ])("while Remove team %s, a cut-short save's Save again is off", async (_, removing) => {
    const error = "saved the Handbook team, but 1 task still needs moving: save again to finish (task-1: busy)";
    vi.mocked(api)
      .mockRejectedValueOnce(new ApiError(error, 409, { error, version: "v3" }))
      .mockReturnValueOnce(new Promise(() => {}));
    afterReload = { "/api/agents": [...ROSTER, ...DOCS.map((a) => ({ ...a, team: "Handbook", teamVersion: "v3" }))] };
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    await save();
    await settle();
    const again = within(summary()).getByRole("button", { name: "Save again" });
    expect(again.hasAttribute("disabled")).toBe(false);
    fireEvent.click(removeButton());
    if (removing) await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));

    expect(again.hasAttribute("disabled")).toBe(true);
    if (removing) return;
    fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Keep" }));
    expect(again.hasAttribute("disabled")).toBe(false);
  });

  test("Back with changes while Remove team asks asks only about the changes; Keep editing asks about removing again", async () => {
    openAt("/tasks", "/agents");
    fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));
    typeInto("Team name", "Handbook");
    fireEvent.click(removeButton("body"));
    await back();

    expect(removeQuestion("body")).toBeNull();
    expect(removeQuestion("footer")).toBeNull();
    const question = inSheet().getByRole("group", { name: "Discard your changes to Docs?" });
    fireEvent.click(within(question).getByRole("button", { name: "Keep editing" }));
    expect(inSheet().queryByRole("group", { name: "Discard your changes to Docs?" })).toBeNull();
    expect(document.activeElement).toBe(within(removeQuestion("body")!).getByRole("button", { name: "Keep" }));
  });
});

describe("Edit team: history", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });
  const editFromThePage = () => fireEvent.click(screen.getByRole("button", { name: "Edit team, Docs" }));

  test("opened from the page, it has an entry of its own, which Close goes back over", async () => {
    openAt("/tasks", "/agents");
    editFromThePage();
    expect(address()).toBe("/agents?team=Docs");
    fireEvent.click(inSheet().getByRole("button", { name: "Close" }));
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("saving goes back over its entry too", async () => {
    vi.mocked(api).mockResolvedValueOnce({ team: "Docs", version: "v2" });
    openAt("/tasks", "/agents");
    editFromThePage();
    fireEvent.click(docsRow("writer"));
    typeInto("Description", "Drafts.");
    await save();
    expect(address()).toBe("/agents");
    await back();
    expect(address()).toBe("/tasks");
  });

  test("Back with no changes closes it", async () => {
    openAt("/tasks", "/agents");
    editFromThePage();
    await back();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("Back with changes keeps it open and asks in the footer, focus on Keep editing", async () => {
    openAt("/tasks", "/agents");
    editFromThePage();
    typeInto("Team name", "Handbook");
    await back();

    expect(address()).toBe("/agents?team=Docs");
    const question = inSheet().getByRole("group", { name: "Discard your changes to Docs?" });
    expect(question.textContent).toContain("The team renamed. They aren't saved.");
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep editing" }));
    await act(async () => fireEvent.click(within(question).getByRole("button", { name: "Discard" })));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(address()).toBe("/agents");
  });

  test("opened from a link, closing takes the panel off the address instead", async () => {
    openAt("/tasks", "/agents?team=Docs");
    fireEvent.click(inSheet().getByRole("button", { name: "Cancel" }));
    expect(address()).toBe("/agents");
    expect(router.state.historyAction).toBe("REPLACE");
    await back();
    expect(address()).toBe("/tasks");
  });
});

describe("Edit team: links out of the panel", () => {
  beforeEach(() => {
    reads["/api/agents"] = [...ROSTER, ...DOCS];
  });
  const pageLink = () => inSheet().getByRole("link", { name: "Open writer's page" });

  test("an open row says what its page has, and with no changes its link goes straight there", async () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    expect(pageLink().parentElement!.textContent).toBe("Knowledge, where it runs and a taller prompt editor: Open writer's page");
    await act(async () => fireEvent.click(pageLink()));
    expect(address()).toBe("/agents/writer");
  });

  test("with changes it asks first, in the footer; Keep editing stays, Discard and open goes", async () => {
    openAt("/agents?team=Docs");
    fireEvent.click(docsRow("writer"));
    typeInto("Description", "Drafts.");
    pageLink().focus();
    await act(async () => fireEvent.click(pageLink()));

    expect(address()).toBe("/agents?team=Docs");
    const question = () => inSheet().getByRole("group", { name: "Open writer's page and discard your changes?" });
    expect(question().textContent).toContain("Save first to keep them; the page shows what is saved.");
    expect(document.activeElement).toBe(within(question()).getByRole("button", { name: "Keep editing" }));
    fireEvent.click(within(question()).getByRole("button", { name: "Keep editing" }));
    expect(document.activeElement).toBe(pageLink());
    expect((inSheet().getByLabelText("Description") as HTMLInputElement).value).toBe("Drafts.");

    await act(async () => fireEvent.click(pageLink()));
    await act(async () => fireEvent.click(within(question()).getByRole("button", { name: "Discard and open" })));
    expect(address()).toBe("/agents/writer");
  });

  test("a task a refusal names asks the same with changes", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError(STILL_NEEDED, 409));
    openAt("/agents?team=Docs");
    typeInto("Team name", "Handbook");
    fireEvent.click(removeButton());
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("button", { name: "Remove" })));
    await act(async () => fireEvent.click(within(removeQuestion()!).getByRole("link", { name: "task-182" })));

    const question = inSheet().getByRole("group", { name: "Open task-182 and discard your changes?" });
    await act(async () => fireEvent.click(within(question).getByRole("button", { name: "Discard and open" })));
    expect(address()).toBe("/tasks/task-182");
  });

  test("a new agent has no page yet, in Edit or in a new team", () => {
    openAt("/agents?team=Docs&add=agent");
    expect(inSheet().getByText("Knowledge and where it runs: on its own page, once you save.")).toBeTruthy();
    expect(inSheet().queryByRole("link", { name: /^Open .*'s page$/ })).toBeNull();
  });

  test("in New team every agent's page waits for the team", () => {
    openAt("/agents?team=new");
    expect(within(panel()).getByText("Knowledge and where it runs: on its own page, once the team is created.")).toBeTruthy();
  });
});
