import type { ReactElement } from "react";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { render as renderIn, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Manifest } from "@/features";
import type { AskVisionaries } from "@/features/ideas/visionaries";
import { SettingsPage } from "./SettingsPage";

// The page reads only the manifest; the test answers it, and asking the
// visionaries is a fresh install's: a template, no team yet.
let manifest: Manifest | undefined;
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
vi.mock("@/features/ideas/visionaries", () => ({ useAskVisionaries: () => FRESH }));

const IDEAS_OFF = { name: "ideas", enabled: false, visible: false, why: "add the Visionaries team to get ideas" };
const IDEAS_ON = { name: "ideas", enabled: true, visible: true, why: "the Visionaries team suggests ideas", badge: 3 };

const render = (page: ReactElement) => renderIn(<MemoryRouter>{page}</MemoryRouter>);

beforeEach(() => {
  manifest = undefined;
});

describe("SettingsPage", () => {
  test("on a fresh install, lists Ideas with its why and Ask the visionaries", () => {
    manifest = { features: [IDEAS_OFF], unknown: [] };
    render(<SettingsPage />);
    expect(screen.getByText("Ideas")).toBeTruthy();
    expect(screen.getByText("add the Visionaries team to get ideas")).toBeTruthy();
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
});
