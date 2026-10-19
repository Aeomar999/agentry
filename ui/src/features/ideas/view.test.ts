import { describe, expect, test } from "vitest";
import { canAsk, ideasView } from "./view";

const none = { loaded: true, failed: false, count: 0 };
const some = { loaded: true, failed: false, count: 2 };
const loading = { read: false, ready: false, has: false, loadError: "" };
const failed = { read: false, ready: false, has: false, loadError: "502 Bad Gateway" };
const noTeam = { read: true, ready: true, has: false, loadError: "" };
const noTemplate = { read: true, ready: false, has: false, loadError: "" };

describe("ideasView", () => {
  test("waits for the ideas and the teams", () => {
    expect(ideasView({ loaded: false, failed: false, count: 0 }, noTeam)).toBe("waiting");
    expect(ideasView(none, loading)).toBe("waiting");
    expect(ideasView(some, loading)).toBe("waiting");
  });

  test("is off with no ideas and no team, and on with the team", () => {
    expect(ideasView(none, noTeam)).toBe("off");
    expect(ideasView(none, { ...noTeam, has: true })).toBe("on");
  });

  test("is on with ideas and no team", () => {
    expect(ideasView(some, noTeam)).toBe("on");
  });

  test("settles when the teams can't be read, on with ideas", () => {
    expect(ideasView(none, failed)).toBe("unknown");
    expect(ideasView(some, failed)).toBe("on");
  });

  test("goes by the teams it read before a reload failed", () => {
    expect(ideasView(none, { ...noTeam, loadError: "502 Bad Gateway" })).toBe("off");
  });

  test("settles when no template turns Ideas on, on with ideas", () => {
    expect(ideasView(none, noTemplate)).toBe("unavailable");
    expect(ideasView(some, noTemplate)).toBe("on");
  });

  test("is on when the ideas can't be read", () => {
    expect(ideasView({ loaded: false, failed: true, count: 0 }, loading)).toBe("on");
  });
});

describe("canAsk", () => {
  test("offers the button while the teams load or can't be read, and once a template turns Ideas on", () => {
    expect(canAsk(loading)).toBe(true);
    expect(canAsk(failed)).toBe(true);
    expect(canAsk(noTeam)).toBe(true);
  });

  test("hides it when no template turns Ideas on", () => {
    expect(canAsk(noTemplate)).toBe(false);
  });
});
