import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { Banner } from "./Banner";

describe("Banner", () => {
  // The plain tones fall under 4.5:1 for text in light; the -ink tokens pass.
  test.each(["danger", "warning", "info", "success"] as const)("writes a %s title in its ink", (tone) => {
    render(<Banner tone={tone} title="Couldn't save the team" />);
    const classes = screen.getByText("Couldn't save the team").classList;
    expect(classes).toContain(`text-${tone}-ink`);
    expect(screen.getByRole("alert").classList).toContain(`text-${tone}`);
  });
});
