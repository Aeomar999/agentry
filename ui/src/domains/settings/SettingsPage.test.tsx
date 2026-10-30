import { type ReactElement, useEffect, useState } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { fireEvent, render as renderIn, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { FeaturesReply } from "@/api/types";
import type { Extensions } from "@/extensions";
import type { AskVisionaries } from "@/extensions/ideas/visionaries";
import { SettingsPage } from "./SettingsPage";

// The page reads only the manifest; the test answers it, and asking the
// visionaries is a fresh install's: a template, no team yet. Beside Ideas,
// the page knows Quiet, an extension with no button that turns it on.
let manifest: FeaturesReply | undefined;
const { QUIET } = vi.hoisted(() => ({ QUIET: { name: "quiet", label: "Quiet" } }));
vi.mock("@/extensions", async (importOriginal) => {
  const real = await importOriginal<typeof import("@/extensions")>();
  const list = [...real.EXTENSIONS, QUIET];
  return { ...real, EXTENSIONS: list, hiddenExtensions: (extensions: Extensions) => real.hiddenExtensions(extensions, list) };
});
vi.mock("@/live/use-resource", () => ({ useResource: () => ({ data: manifest, error: "", position: 1, reload: async () => {} }) }));

const FRESH: AskVisionaries = {
  read: true,
  ready: true,
  loadError: "",
  has: false,
  team: "Visionaries",
  adding: false,
  error: "",
  missingProfiles: [],
  ask: () => {},
  askAddingProfiles: () => {},
};
let useAskVisionaries = (): AskVisionaries => FRESH;
vi.mock("@/extensions/ideas/visionaries", () => ({ useAskVisionaries: () => useAskVisionaries() }));

const IDEAS_OFF = { name: "ideas", enabled: false, visible: false, why: "add a team from the Visionaries template to get ideas" };
const IDEAS_ON = { name: "ideas", enabled: true, visible: true, why: "the Visionaries team suggests ideas", badge: 3 };

const render = (page: ReactElement) => renderIn(<MemoryRouter>{page}</MemoryRouter>);

beforeEach(() => {
  manifest = undefined;
  useAskVisionaries = () => FRESH;
});

describe("SettingsPage", () => {
  test("on a fresh install, lists Ideas with its why and Ask the visionaries", () => {
    manifest = { features: [IDEAS_OFF], unknown: [] };
    render(<SettingsPage />);
    expect(screen.getByText("Ideas")).toBeTruthy();
    expect(screen.getByText("add a team from the Visionaries template to get ideas")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Ask the visionaries" })).toBeTruthy();
    expect(screen.queryByText("Every feature is on.")).toBeNull();
  });

  test("doesn't list a visible feature, and says nothing is off", () => {
    manifest = { features: [IDEAS_ON], unknown: null };
    render(<SettingsPage />);
    expect(screen.queryByText("Ideas")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Every feature is on.")).toBeTruthy();
  });

  test("says nothing until the manifest is read", () => {
    render(<SettingsPage />);
    expect(screen.queryByText("Every feature is on.")).toBeNull();
  });

  test("shows one tally line per prefix and per type", () => {
    manifest = {
      features: [IDEAS_ON],
      unknown: [
        { prefix: "idea-", streams: 12, events: 31, last: 48211 },
        { type: "git.pr_opened", events: 4, last: 48190 },
      ],
    };
    render(<SettingsPage />);
    expect(screen.getByText("31 events from a feature this build doesn't have: idea-")).toBeTruthy();
    expect(screen.getByText("4 events of a type this build doesn't know: git.pr_opened")).toBeTruthy();
  });

  test("ignores a feature only the server knows", () => {
    manifest = { features: [IDEAS_OFF, { name: "nowhere", enabled: false, visible: false, why: "not in this page" }], unknown: [] };
    render(<SettingsPage />);
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.queryByText("not in this page")).toBeNull();
  });

  test("shows a feature with no button by its label and why alone", () => {
    manifest = { features: [IDEAS_ON, { name: "quiet", enabled: false, visible: false, why: "nothing turns it on here" }], unknown: [] };
    render(<SettingsPage />);
    expect(screen.getByText("Quiet")).toBeTruthy();
    expect(screen.getByText("nothing turns it on here")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  test("keeps Ideas' button mounted when Ideas turns on before its team shows, so New task opens", () => {
    // Like the real hook: asking waits in the button's own state for the
    // team to show, then opens New task.
    const newTask = vi.fn();
    let teamShows = false;
    useAskVisionaries = () => {
      const [opening, setOpening] = useState(false);
      useEffect(() => {
        if (!opening || !teamShows) return;
        setOpening(false);
        newTask();
      });
      return { ...FRESH, adding: opening, ask: () => setOpening(true) };
    };
    manifest = { features: [IDEAS_OFF], unknown: [] };
    const page = render(<SettingsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Ask the visionaries" }));

    manifest = { features: [IDEAS_ON], unknown: [] };
    page.rerender(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    );
    expect(screen.getByText("Ideas")).toBeTruthy();

    teamShows = true;
    page.rerender(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    );
    expect(newTask).toHaveBeenCalledTimes(1);
  });

  test("opens redacting a text everywhere from its link, and closes it", () => {
    manifest = { features: [IDEAS_ON], unknown: null };
    render(<SettingsPage />);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("link", { name: "Redact a text everywhere" }));
    expect(screen.getByRole("dialog", { name: "Redact a text everywhere" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
