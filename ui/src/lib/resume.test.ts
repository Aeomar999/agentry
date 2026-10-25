import { describe, expect, it } from "vitest";
import { byItsWork, startsEachTurn, startsFresh } from "./resume";

describe("how an agent starts each turn", () => {
  it("is fresh for a reviewer by default and for anyone set to, else going on", () => {
    expect(startsFresh({ column: "review", resume: "" })).toBe(true);
    expect(startsFresh({ column: "", resume: "" })).toBe(false);
    expect(startsFresh({ column: "ship", resume: "" })).toBe(false);
    expect(startsFresh({ column: "", resume: "fresh" })).toBe(true);
    expect(startsFresh({ column: "review", resume: "resume" })).toBe(false);
  });

  it("says why on the agent's page", () => {
    expect(startsEachTurn({ column: "review", resume: "" })).toBe("Fresh (reviewers start fresh)");
    expect(startsEachTurn({ column: "", resume: "fresh" })).toBe("Fresh (set to start fresh)");
    expect(startsEachTurn({ column: "", resume: "" })).toBe("Going on with its session");
    expect(startsEachTurn({ column: "ship", resume: "" })).toBe("Going on with its session");
    expect(startsEachTurn({ column: "review", resume: "resume" })).toBe("Going on with its session (set to)");
  });

  it("tells a draft what By its work comes to, from its column or whether it reviews", () => {
    expect(byItsWork({ column: "review", resume: "", coordinates: false })).toBe("Fresh, since its work counts as review.");
    expect(byItsWork({ column: "", resume: "", coordinates: false })).toBe("Going on with its session, since its work counts as building.");
    expect(byItsWork({ column: "ship", resume: "fresh", coordinates: false })).toBe("Going on with its session, since its work counts as shipping.");
    expect(byItsWork({ column: "", resume: "", coordinates: true })).toBe("Going on with its session, since it doesn't review.");
    expect(byItsWork({ column: "review", resume: "", coordinates: true })).toBe("Fresh, since it reviews.");
  });
});
