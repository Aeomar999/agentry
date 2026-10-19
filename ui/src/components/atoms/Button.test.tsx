import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { Button } from "./Button";

describe("Button", () => {
  // Plain danger is 3.8:1 on light surfaces; danger-ink reads at 4.5:1 in both themes.
  test("writes danger text in danger-ink", () => {
    render(<Button variant="danger">Remove team</Button>);
    const classes = screen.getByRole("button", { name: "Remove team" }).classList;
    expect(classes).toContain("text-danger-ink");
    expect(classes).not.toContain("text-danger");
  });
});
