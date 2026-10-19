import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api, ApiError } from "@/api/client";
import { AcceptIdeaDialog } from "./IdeaDialogs";
import type { Idea } from "./types";

// The client reads the address when it loads, and there is none here.
vi.mock("@/api/client", () => ({
  api: vi.fn(),
  ApiError: class extends Error {
    constructor(
      message: string,
      readonly status: number,
      readonly body?: Record<string, unknown>,
    ) {
      super(message);
    }
  },
}));
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string | null) => ({ data: path === "/api/config" ? { budget: 1000 } : undefined, error: "" }),
}));

const idea = {
  id: "idea-2",
  title: "Make a ship the user chose push, and count it shipped only when it pushed",
  kind: "",
  epic: "",
} as Idea;

// like is the 409 for an idea that looks like task-2.
const like = new ApiError('idea-2 looks like the open task task-2 "Make a ship…"', 409, {
  error: 'idea-2 looks like the open task task-2 "Make a ship…"',
  duplicate: { id: "task-2", title: "Make a ship the user chose actually push", column: "backlog" },
});

function open(onClose = () => {}) {
  render(
    <MemoryRouter>
      <AcceptIdeaDialog idea={idea} onClose={onClose} />
    </MemoryRouter>,
  );
}

describe("AcceptIdeaDialog", () => {
  test("shows the open task an idea looks like, and makes another only when you say anyway", async () => {
    const calls = vi.mocked(api);
    calls.mockReset();
    calls.mockRejectedValueOnce(like).mockResolvedValueOnce({ task: "task-3", column: "backlog", accepted: true, refused: "" });
    open();

    fireEvent.click(screen.getByRole("button", { name: "Accept as a task" }));
    const link = await screen.findByRole("link", { name: "Make a ship the user chose actually push" });
    expect(link.getAttribute("href")).toBe("/tasks/task-2");
    expect(calls).toHaveBeenCalledTimes(1);
    expect(calls.mock.calls[0][2]).not.toHaveProperty("anyway");

    fireEvent.click(screen.getByRole("button", { name: "Make another anyway" }));
    expect(await screen.findByRole("link", { name: "task-3" })).toBeTruthy();
    expect(calls).toHaveBeenCalledTimes(2);
    expect(calls.mock.calls[1][2]).toMatchObject({ kind: "task", anyway: true });
  });

  test("Cancel leaves the idea open, sending nothing more", async () => {
    const calls = vi.mocked(api);
    calls.mockReset();
    calls.mockRejectedValueOnce(like);
    const onClose = vi.fn();
    open(onClose);

    fireEvent.click(screen.getByRole("button", { name: "Accept and start" }));
    await screen.findByRole("button", { name: "Make another anyway" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
    expect(calls).toHaveBeenCalledTimes(1);
  });

  test("any other refusal says why, as before", async () => {
    const calls = vi.mocked(api);
    calls.mockReset();
    calls.mockRejectedValueOnce(new ApiError("idea idea-2 was dismissed already", 409));
    open();

    fireEvent.click(screen.getByRole("button", { name: "Accept as a task" }));
    expect(await screen.findByText("idea idea-2 was dismissed already")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Make another anyway" })).toBeNull();
  });
});
