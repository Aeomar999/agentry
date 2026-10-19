import { describe, expect, test } from "vitest";
import { render } from "@testing-library/react";
import { Badge, type BadgeVariant } from "./Badge";

function badge(variant?: BadgeVariant) {
  const { container } = render(<Badge variant={variant}>label</Badge>);
  return container.firstElementChild as HTMLElement;
}

describe("Badge", () => {
  // The state colours themselves fall under 4.5:1 on their soft fills; the
  // -ink tokens are the ones tuned to pass in light and dark.
  test.each([
    ["success", "bg-success-soft", "text-success-ink"],
    ["danger", "bg-danger-soft", "text-danger-ink"],
    ["warning", "bg-warning-soft", "text-warning-ink"],
    ["info", "bg-info-soft", "text-info-ink"],
    ["brand", "bg-brand-soft", "text-brand-ink"],
  ] as const)("writes %s text in its AA ink on its soft fill", (variant, fill, ink) => {
    const classes = badge(variant).classList;
    expect(classes).toContain(fill);
    expect(classes).toContain(ink);
    expect(classes).not.toContain(`text-${variant}`);
  });

  test("keeps the default badge in secondary ink on surface-3", () => {
    const classes = badge().classList;
    expect(classes).toContain("bg-surface-3");
    expect(classes).toContain("text-ink-secondary");
  });
});
