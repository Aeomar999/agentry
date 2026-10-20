import { useState } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { api, ApiError, send } from "@/api/client";
import type { TeamTemplate } from "@/api/types";
import { AddTeam } from "./AddTeam";

vi.mock("@/api/client", async (actual) => ({ ...(await actual<typeof import("@/api/client")>()), api: vi.fn(), send: vi.fn() }));

// The toasts the list shows, as calls.
const { notify } = vi.hoisted(() => ({ notify: vi.fn() }));
vi.mock("@/components/organisms/Toasts", async (actual) => ({
  ...(await actual<typeof import("@/components/organisms/Toasts")>()),
  useToast: () => notify,
}));

const template = (team: string, given: Partial<TeamTemplate> = {}): TeamTemplate => ({
  id: team.toLowerCase().replace(/ /g, "-"),
  team,
  description: `The ${team} team.`,
  features: [],
  userMade: false,
  agents: [
    {
      name: `${team.toLowerCase().replace(/ /g, "-")}-lead`,
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
  ...given,
});
const TEMPLATES = [template("Software"), template("Writing"), template("Visionaries")];

// Yours, saved this year on the day given (in October), from the team given.
const yours = (team: string, day: number, from = team) =>
  template(team, { userMade: true, savedAt: new Date(new Date().getFullYear(), 9, day, 9).toISOString(), from });
const PAPERS = yours("Papers", 12, "Reading");
const ORDERS = yours("Orders", 15);
const DESK = yours("Desk", 3);

const refused = new ApiError("the policy lacks read-mostly", 409, { missingProfiles: ["read-mostly"] });

let onBuild = vi.fn();
let onAdded = vi.fn();
let onTemplatesChanged = vi.fn();

// list draws the list over templates; after a delete or an import the
// templates read again are next, when given.
function list(templates: TeamTemplate[], next?: TeamTemplate[]) {
  function Harness() {
    const [shown, setShown] = useState(templates);
    return (
      <AddTeam
        templates={shown}
        existing={["Writing"]}
        policyHash="h1"
        onBuild={onBuild}
        onAdded={onAdded}
        onTemplatesChanged={async () => {
          onTemplatesChanged();
          if (next) setShown(next);
        }}
      />
    );
  }
  render(<Harness />);
}

const card = (name: string) => screen.getByRole("group", { name });
const click = (button: HTMLElement) => act(async () => fireEvent.click(button));

beforeEach(() => {
  vi.mocked(api).mockReset();
  vi.mocked(send).mockReset();
  notify.mockReset();
  onBuild = vi.fn();
  onAdded = vi.fn();
  onTemplatesChanged = vi.fn();
  Element.prototype.scrollIntoView = vi.fn();
});

describe("AddTeam: the groups", () => {
  test("without templates of yours there is no Your templates", () => {
    list(TEMPLATES);
    expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual(["Build your own", "Built in"]);
  });

  test("yours come between Build your own and Built in, newest first, each saying when and where it was saved", () => {
    list([...TEMPLATES, PAPERS, ORDERS, DESK]);
    expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual(["Build your own", "Your templates", "Built in"]);
    const cards = screen.getAllByRole("group").map((group) => group.getAttribute("aria-label"));
    expect(cards).toEqual(["A team of your own", "Orders", "Papers", "Desk", "Software", "Writing", "Visionaries"]);
    expect(card("Papers").textContent).toContain("Saved 12 Oct from the Reading team.");
    expect(card("Orders").textContent).toContain("Saved 15 Oct from the Orders team.");
  });

  test("only yours can be deleted or exported", () => {
    list([...TEMPLATES, PAPERS]);
    expect(within(card("Papers")).getByRole("button", { name: "Delete template, Papers" })).toBeTruthy();
    expect(within(card("Papers")).getByRole("link", { name: "Export, Papers" }).getAttribute("href")).toBe(
      "/api/teams/templates/papers/export",
    );
    expect(within(card("Software")).queryByRole("button", { name: /Delete template/ })).toBeNull();
    expect(within(card("Software")).queryByRole("link")).toBeNull();
  });

  test("every card's actions are named with it, and its coordinator chip uses the brand ink", () => {
    list([...TEMPLATES, PAPERS]);
    expect(within(card("Software")).getByRole("button", { name: "Add this team, Software" })).toBeTruthy();
    expect(within(card("Writing")).getByRole("button", { name: "Add another, Writing" }).textContent).toBe("Add another");
    expect(within(card("Papers")).getByRole("button", { name: "Change it first, Papers" })).toBeTruthy();
    for (const name of ["Software", "Papers"]) {
      expect(within(card(name)).getByText(`${name.toLowerCase()}-lead`).className).toContain("text-brand-ink");
    }
  });
});

describe("AddTeam: a team refused for its profiles", () => {
  test("the offer shows in the refused team's card, scrolled into view", async () => {
    vi.mocked(api).mockRejectedValueOnce(refused);
    list(TEMPLATES);
    await click(within(card("Visionaries")).getByRole("button", { name: "Add this team, Visionaries" }));

    const offer = within(card("Visionaries")).getByRole("alert");
    expect(offer.textContent).toContain("The Visionaries team needs the read-mostly profile");
    expect(screen.getAllByRole("alert")).toEqual([offer]); // in no other card, nor above them all
    expect(within(offer).getByRole("button", { name: "Add the profile and the team" })).toBeTruthy();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
  });

  test("its button adds the profile with the team, against the policy the page saw", async () => {
    vi.mocked(api).mockRejectedValueOnce(refused).mockResolvedValueOnce({});
    list(TEMPLATES);
    await click(within(card("Visionaries")).getByRole("button", { name: "Add this team, Visionaries" }));
    await click(within(card("Visionaries")).getByRole("button", { name: "Add the profile and the team" }));

    expect(vi.mocked(api).mock.calls[1]).toEqual(["POST", "/api/teams", { template: "Visionaries", addMissingProfiles: true, hash: "h1" }]);
    expect(onAdded).toHaveBeenCalledWith("Visionaries");
  });
});

describe("AddTeam: a template of yours", () => {
  test("is added by its id", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    list([...TEMPLATES, PAPERS]);
    await click(within(card("Papers")).getByRole("button", { name: "Add this team, Papers" }));

    expect(vi.mocked(api)).toHaveBeenCalledWith("POST", "/api/teams", { template: "papers" });
    expect(notify).toHaveBeenCalledWith("success", "Added the Papers team", "Pick it on New task; its coordinator takes the task.");
    expect(onAdded).toHaveBeenCalledWith("Papers");
  });

  test("refused for its profiles, offers them in its own card and adds it with them", async () => {
    vi.mocked(api).mockRejectedValueOnce(refused).mockResolvedValueOnce({});
    list([...TEMPLATES, PAPERS]);
    await click(within(card("Papers")).getByRole("button", { name: "Add this team, Papers" }));
    expect(screen.getAllByRole("alert")).toEqual([within(card("Papers")).getByRole("alert")]);
    await click(within(card("Papers")).getByRole("button", { name: "Add the profile and the team" }));

    expect(vi.mocked(api).mock.calls[1]).toEqual(["POST", "/api/teams", { template: "papers", addMissingProfiles: true, hash: "h1" }]);
    expect(onAdded).toHaveBeenCalledWith("Papers");
  });

  test("Change it first opens the panel copying it", async () => {
    list([...TEMPLATES, PAPERS]);
    await click(within(card("Papers")).getByRole("button", { name: "Change it first, Papers" }));
    expect(onBuild).toHaveBeenCalledWith("papers");
  });
});

describe("AddTeam: deleting a template of yours", () => {
  const ask = (name: string) => click(within(card(name)).getByRole("button", { name: `Delete template, ${name}` }));

  test("asks in its card, focus on Keep, in place of its actions", async () => {
    list([...TEMPLATES, PAPERS]);
    await ask("Papers");
    const question = within(card("Papers")).getByRole("group", { name: "Delete the Papers template?" });
    expect(question.textContent).toContain("Teams you made from it stay as they are.");
    expect(document.activeElement).toBe(within(question).getByRole("button", { name: "Keep" }));
    expect(within(card("Papers")).queryByRole("button", { name: "Add this team, Papers" })).toBeNull();
    expect(within(card("Papers")).queryByRole("button", { name: /Delete template/ })).toBeNull();
  });

  test("Keep, or Esc, puts the card back as it was, focus on Delete template", async () => {
    list([...TEMPLATES, PAPERS]);
    await ask("Papers");
    await click(within(card("Papers")).getByRole("button", { name: "Keep" }));
    expect(document.activeElement).toBe(within(card("Papers")).getByRole("button", { name: "Delete template, Papers" }));

    await ask("Papers");
    const escape = new KeyboardEvent("keydown", { key: "Escape", bubbles: true });
    const closed = vi.fn(); // the dialog closes on an Esc that reaches the window
    window.addEventListener("keydown", closed);
    await act(async () => within(card("Papers")).getByRole("button", { name: "Keep" }).dispatchEvent(escape));
    window.removeEventListener("keydown", closed);
    expect(closed).not.toHaveBeenCalled();
    expect(within(card("Papers")).queryByRole("group", { name: "Delete the Papers template?" })).toBeNull();
    expect(document.activeElement).toBe(within(card("Papers")).getByRole("button", { name: "Delete template, Papers" }));
    expect(vi.mocked(api)).not.toHaveBeenCalled();
  });

  test("Delete deletes it, reads the list again and moves focus to the next card's Add this team", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    list([...TEMPLATES, ORDERS, PAPERS], [...TEMPLATES, PAPERS]);
    await ask("Orders");
    await click(within(card("Orders")).getByRole("button", { name: "Delete" }));

    expect(vi.mocked(api)).toHaveBeenCalledWith("DELETE", "/api/teams/templates/orders");
    expect(notify).toHaveBeenCalledWith("success", "Deleted the Orders template");
    expect(onTemplatesChanged).toHaveBeenCalled();
    expect(screen.queryByRole("group", { name: "Orders" })).toBeNull();
    expect(document.activeElement).toBe(within(card("Papers")).getByRole("button", { name: "Add this team, Papers" }));
  });

  test("the last of yours deleted, the one before it takes focus", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    list([...TEMPLATES, ORDERS, PAPERS], [...TEMPLATES, ORDERS]);
    await ask("Papers");
    await click(within(card("Papers")).getByRole("button", { name: "Delete" }));
    expect(document.activeElement).toBe(within(card("Orders")).getByRole("button", { name: "Add this team, Orders" }));
  });

  test("with none of yours left, Build a team takes focus and Your templates goes", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    list([...TEMPLATES, PAPERS], TEMPLATES);
    await ask("Papers");
    await click(within(card("Papers")).getByRole("button", { name: "Delete" }));
    expect(screen.queryByRole("heading", { name: "Your templates" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Build a team" }));
  });

  test("while it runs, says so and nothing else in the list can start", async () => {
    let answer = (_: unknown) => {};
    vi.mocked(api).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    list([...TEMPLATES, PAPERS], TEMPLATES);
    await ask("Papers");
    await click(within(card("Papers")).getByRole("button", { name: "Delete" }));

    expect(within(card("Papers")).getByRole("button", { name: "Deleting…" })).toHaveProperty("disabled", true);
    expect(within(card("Papers")).getByRole("button", { name: "Keep" })).toHaveProperty("disabled", true);
    for (const name of ["Build a team", "Import a template", "Add this team, Software", "Change it first, Software"]) {
      expect(screen.getByRole("button", { name })).toHaveProperty("disabled", true);
    }
    await act(async () => answer({}));
  });

  test("a failure says so in the card, with the server's words, and the question stays", async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError("there is no saved template papers", 404));
    list([...TEMPLATES, PAPERS]);
    await ask("Papers");
    await click(within(card("Papers")).getByRole("button", { name: "Delete" }));

    const question = within(card("Papers")).getByRole("group", { name: "Delete the Papers template?" });
    expect(question.textContent).toContain("Couldn't delete the template");
    expect(question.textContent).toContain("there is no saved template papers");
    expect(within(question).getByRole("button", { name: "Delete" })).toHaveProperty("disabled", false);
    expect(notify).not.toHaveBeenCalled();
  });
});

describe("AddTeam: importing a template", () => {
  const file = new File(['{"id":"papers"}'], "papers.json", { type: "application/json" });
  const pick = () => act(async () => fireEvent.change(screen.getByTestId("template-file"), { target: { files: [file] } }));

  test("Import a template opens a picker for .json files", async () => {
    list(TEMPLATES);
    const input = screen.getByTestId("template-file") as HTMLInputElement;
    const opened = vi.spyOn(input, "click");
    await click(screen.getByRole("button", { name: "Import a template" }));
    expect(opened).toHaveBeenCalled();
    expect(input.accept).toBe(".json,application/json");
  });

  test("an import is sent as it is, said in a toast, and the list read again", async () => {
    vi.mocked(send).mockResolvedValueOnce(new Response(JSON.stringify(PAPERS)));
    list(TEMPLATES, [...TEMPLATES, PAPERS]);
    await pick();

    expect(vi.mocked(send).mock.calls[0][0]).toBe("/api/teams/templates/import");
    expect(vi.mocked(send).mock.calls[0][1].body).toBe(file);
    expect(notify).toHaveBeenCalledWith("success", "Imported Papers to your templates");
    expect(onTemplatesChanged).toHaveBeenCalled();
    expect(card("Papers")).toBeTruthy();
  });

  test("a refused import says why under the button, a name you have in the server's words", async () => {
    const taken = "You have a template called Papers already. Pick another name.";
    vi.mocked(send).mockRejectedValueOnce(new ApiError(taken, 409));
    list([...TEMPLATES, PAPERS]);
    await pick();

    const failure = within(card("A team of your own")).getByRole("alert");
    expect(failure.textContent).toBe(`Couldn't import the template${taken}`);
    expect(onTemplatesChanged).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Import a template" })).toHaveProperty("disabled", false);
  });
});
