import { describe, expect, test } from "vitest";
import { render } from "@testing-library/react";
import { AgentColorsProvider } from "@/live/agent-colors";
import type { Agent } from "@/api/types";
import { Avatar } from "./Avatar";

const engineer = { name: "engineer", color: "teal" } as Agent;

function chip(name: string, size?: 20 | 28) {
  const { container } = render(
    <AgentColorsProvider agents={[engineer]}>
      <Avatar name={name} size={size} />
    </AgentColorsProvider>,
  );
  return container.firstElementChild as HTMLElement;
}

describe("Avatar", () => {
  test("draws an agent's letter on a tint of its colour, the letter darker in light", () => {
    const span = chip("engineer");
    expect(span.textContent).toBe("E");
    expect(span.getAttribute("aria-hidden")).toBe("true");
    expect(span.getAttribute("style")).toContain("var(--color-agent-teal) var(--chip-letter), var(--color-ink-primary)");
    expect(span.getAttribute("style")).toContain("var(--color-agent-teal) var(--chip-tint), transparent");
  });

  test.each([
    ["you", "Y"],
    ["Agentry", "A"],
  ])("draws %s neutral", (name, letter) => {
    const span = chip(name);
    expect(span.textContent).toBe(letter);
    expect(span.className).toContain("bg-surface-3");
    expect(span.className).toContain("text-ink-secondary");
    expect(span.getAttribute("style")).toBeNull();
  });

  test("comes in 28px, the chat's, and 20px", () => {
    expect(chip("engineer").className).toContain("size-7");
    expect(chip("engineer", 20).className).toContain("size-5");
  });
});
