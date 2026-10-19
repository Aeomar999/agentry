import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import type { Turn } from "@/api/types";
import { HandoffPath } from "./HandoffPath";

const turn = (id: string, agent: string) => ({ id, agent, ended: true, issue: "" }) as Turn;

const TURNS = [turn("turn-1", "coordinator"), turn("turn-2", "engineer"), turn("turn-3", "engineer"), turn("turn-4", "reviewer")];

describe("HandoffPath", () => {
  test("hides its chevrons from screen readers", () => {
    const { container } = render(<HandoffPath turns={TURNS} coordinator="coordinator" done onOpenTurn={() => {}} />);
    const chevrons = container.querySelectorAll("svg.lucide-chevron-right");
    expect(chevrons).toHaveLength(2);
    for (const chevron of chevrons) expect(chevron.getAttribute("aria-hidden")).toBe("true");
  });

  test("counts an agent's turns in a row in muted ink, not faint", () => {
    render(<HandoffPath turns={TURNS} coordinator="coordinator" done={false} onOpenTurn={() => {}} />);
    const count = screen.getByText("×2");
    expect(count.className).toContain("text-ink-muted");
    expect(count.className).not.toContain("text-ink-faint");
  });
});
