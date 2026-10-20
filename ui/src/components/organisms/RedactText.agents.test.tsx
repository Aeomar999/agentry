import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { previewAgentsRedaction, previewRedaction, redact, redactAgents } from "@/api/privacy";
import { dateTime } from "@/lib/time";
import { RedactText } from "./RedactText";

vi.mock("@/api/privacy", () => ({ previewRedaction: vi.fn(), redact: vi.fn(), previewAgentsRedaction: vi.fn(), redactAgents: vi.fn() }));

// RedactText for the agents' roster and what they learned: the same
// count-first flow as a task's, through the agents' routes.

const SECRET = "sk-live-0123456789";
const AT = "2026-10-20T09:30:00Z";

let onRedacted = vi.fn();

beforeEach(() => {
  vi.mocked(previewAgentsRedaction).mockReset();
  vi.mocked(redactAgents).mockReset();
  onRedacted = vi.fn();
});

function ask() {
  render(<RedactText agents marked={SECRET} onClose={vi.fn()} onRedacted={onRedacted} />);
}

const button = (name: string) => screen.getByRole("button", { name });

async function count() {
  await act(async () => fireEvent.click(button("Count the rows")));
}

describe("RedactText, for the agents and what they learned", () => {
  test("counts the rows first, redacts the rows you confirmed, then says what it did with no worktree to remove", async () => {
    vi.mocked(previewAgentsRedaction).mockResolvedValue(2);
    vi.mocked(redactAgents).mockResolvedValue({ stream: "agents", at: AT, rows: 2, warnings: [] });
    ask();
    expect(screen.getByText("Redact a text in the agents and what they learned")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Why, for the log (optional)"), { target: { value: "pasted a key" } });
    await count();

    expect(previewAgentsRedaction).toHaveBeenCalledWith(SECRET);
    expect(screen.getByText(/^2 rows of the agents and what they learned hold this text\./)).toBeTruthy();
    expect(redactAgents).not.toHaveBeenCalled();

    await act(async () => fireEvent.click(button("Redact 2 rows")));
    expect(redactAgents).toHaveBeenCalledWith(SECRET, "pasted a key", 2);
    expect(previewRedaction).not.toHaveBeenCalled();
    expect(redact).not.toHaveBeenCalled();
    expect(onRedacted).toHaveBeenCalled();
    expect(screen.getByText(`Redacted 2 rows of the agents and what they learned at ${dateTime(AT)}.`)).toBeTruthy();
    expect(screen.queryByText(/worktree/)).toBeNull();
  });

  test("a stale count is refused 409, counted again and asked again with the new count", async () => {
    vi.mocked(previewAgentsRedaction).mockResolvedValueOnce(1).mockResolvedValueOnce(2);
    vi.mocked(redactAgents).mockRejectedValueOnce(new ApiError("2 rows hold this text now, not 1: count them again", 409));
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 1 row")));

    expect(redactAgents).toHaveBeenLastCalledWith(SECRET, "", 1);
    expect(previewAgentsRedaction).toHaveBeenCalledTimes(2);
    expect(screen.getByText(/^2 rows of the agents and what they learned hold this text\./)).toBeTruthy();
    expect(screen.getByText(/2 rows hold this text now, not 1/).textContent).toContain("so the rows were counted again");
    expect(onRedacted).not.toHaveBeenCalled();
  });

  test("when no row holds the text any more, goes back to the text with the reason", async () => {
    vi.mocked(previewAgentsRedaction)
      .mockResolvedValueOnce(1)
      .mockRejectedValueOnce(new ApiError("no row of agents holds the secret", 409));
    vi.mocked(redactAgents).mockRejectedValueOnce(new ApiError("no row of agents holds the secret", 409));
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 1 row")));
    expect(screen.getByText("It can't be redacted: no row of agents holds the secret")).toBeTruthy();
  });
});
