import { describe, expect, test } from "vitest";
import { render } from "@testing-library/react";
import type { Agent } from "@/api/types";
import { AuthorBand, authorName } from "./AuthorBand";

const agent = (name: string, fields: Partial<Agent> = {}) => ({ name, coordinates: false, column: "", team: "", ...fields }) as Agent;

function band(name: string, of?: Agent) {
  const { container } = render(<AuthorBand name={name} agent={of} />);
  return container;
}

describe("AuthorBand", () => {
  test("shows the chip, hidden from screen readers, then the name, then team · role", () => {
    const container = band("release", agent("release", { column: "ship" }));
    const chip = container.querySelector("[aria-hidden]")!;
    expect(chip.textContent).toBe("R");
    expect(container.textContent).toBe("Rrelease" + "Software · deployer");
  });

  test("leaves the role out when it is the agent's name, and shows the team", () => {
    expect(band("reviewer", agent("reviewer", { column: "review" })).textContent).toBe("RreviewerSoftware");
  });

  test("names a coordinator's team", () => {
    expect(band("product-lead", agent("product-lead", { coordinates: true, team: "Design" })).textContent).toBe(
      "Pproduct-leadDesign · coordinator",
    );
  });

  test.each(["you", "Agentry"])("shows %s alone, on a neutral chip", (name) => {
    const container = band(name);
    expect(container.textContent).toBe(name[0].toUpperCase() + name);
    expect(container.querySelector("[aria-hidden]")!.className).toContain("bg-surface-3");
  });

  test("puts the caller's facts after the author", () => {
    const { container } = render(
      <AuthorBand name="engineer" agent={agent("engineer")}>
        <span>turn-5</span>
      </AuthorBand>,
    );
    expect(container.textContent).toBe("EengineerSoftwareturn-5");
  });
});

describe("authorName", () => {
  test.each([
    ["release", agent("release", { column: "ship" }), "release, Software team, deployer"],
    ["engineer", agent("engineer"), "engineer, Software team"],
    ["reviewer", agent("reviewer", { column: "review" }), "reviewer, Software team"],
    ["coordinator", agent("coordinator", { coordinates: true, team: "Design" }), "coordinator, Design team"],
    ["you", undefined, "you"],
    ["Agentry", undefined, "Agentry"],
  ])("says %s as %j", (name, of, said) => {
    expect(authorName(name, of)).toBe(said);
  });
});
