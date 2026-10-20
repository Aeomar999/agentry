import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { previewLogRedaction, redactLog } from "@/api/privacy";
import type { LogPreview, LogRedaction } from "@/api/types";
import { dateTime } from "@/lib/time";
import { RedactLog } from "./RedactLog";

vi.mock("@/api/privacy", () => ({ previewLogRedaction: vi.fn(), redactLog: vi.fn() }));

const SECRET = "sk-live-0123456789";
const AT = "2026-10-20T09:30:00Z";

// The preview the server lists: two live tasks, an archived one, the
// agents, and a task whose blob is lost, which can't be redacted.
const PREVIEW: LogPreview = {
  rows: 5,
  streams: [
    { stream: "task-3", rows: 2, archived: false },
    { stream: "task-5", rows: 1, archived: false },
    { stream: "task-6", rows: 1, archived: true },
    { stream: "agents", rows: 1, archived: false },
    { stream: "task-7", rows: 0, archived: true, error: "task-7's archived rows can't be read: its blob is missing" },
  ],
};
const COUNTS = { "task-3": 2, "task-5": 1, "task-6": 1, agents: 1 };

const STALE = "2 rows hold this text now, not 1: count them again";
const LOST = "task-7's archived rows can't be read: its blob is missing";

// The redaction with task-5's count stale and task-7 failing, as the
// server answers it: each stream left among the warnings too.
const PARTLY: LogRedaction = {
  rows: 4,
  streams: [
    { stream: "task-3", at: AT, rows: 2 },
    { stream: "task-6", at: AT, rows: 1 },
    { stream: "agents", at: AT, rows: 1 },
  ],
  failures: [
    { stream: "task-5", reason: STALE, recount: true },
    { stream: "task-7", reason: LOST, recount: false },
  ],
  warnings: [`task-5: ${STALE}`, `task-7: ${LOST}`, "the WAL couldn't be emptied"],
};

beforeEach(() => {
  vi.mocked(previewLogRedaction).mockReset();
  vi.mocked(redactLog).mockReset();
});

const button = (name: string) => screen.getByRole("button", { name });

async function count() {
  render(<RedactLog onClose={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("The text to take out"), { target: { value: SECRET } });
  fireEvent.change(screen.getByLabelText("Why, for the log (optional)"), { target: { value: "pasted everywhere" } });
  await act(async () => fireEvent.click(button("Count the rows")));
}

const row = (stream: string) => screen.getAllByText(stream).map((name) => name.closest("li") as HTMLElement);

describe("RedactLog", () => {
  test("lists each stream's rows first, archived marked and failures named, and redacts nothing yet", async () => {
    vi.mocked(previewLogRedaction).mockResolvedValue(PREVIEW);
    await count();
    expect(previewLogRedaction).toHaveBeenCalledWith(SECRET);
    expect(screen.getByText(/^5 rows hold this text\./)).toBeTruthy();
    expect(within(row("task-3")[0]).getByText("2 rows")).toBeTruthy();
    expect(within(row("task-6")[0]).getByText("archived")).toBeTruthy();
    expect(within(row("task-5")[0]).queryByText("archived")).toBeNull();
    expect(within(row("task-7")[0]).getByText(`can't be redacted: ${LOST}`)).toBeTruthy();
    expect(redactLog).not.toHaveBeenCalled();
  });

  test("sends the rows counted in each stream it can redact, then lists what it did and names each stream left", async () => {
    vi.mocked(previewLogRedaction).mockResolvedValue(PREVIEW);
    vi.mocked(redactLog).mockResolvedValue(PARTLY);
    await count();
    await act(async () => fireEvent.click(button("Redact 5 rows")));

    expect(redactLog).toHaveBeenCalledWith(SECRET, "pasted everywhere", COUNTS);
    expect(screen.getByText(`Redacted 4 rows at ${dateTime(AT)}.`)).toBeTruthy();
    expect(within(row("task-3")[0]).getByText("2 rows")).toBeTruthy();
    expect(within(row("agents")[0]).getByText("1 row")).toBeTruthy();
    expect(screen.getByText("These still hold the text:")).toBeTruthy();
    expect(within(row("task-5")[0]).getByText(STALE)).toBeTruthy();
    expect(within(row("task-7")[0]).getByText(LOST)).toBeTruthy();
    // The warnings say only what the streams left don't.
    expect(screen.getByText("the WAL couldn't be emptied")).toBeTruthy();
    expect(screen.queryByText(`task-5: ${STALE}`)).toBeNull();
    expect(screen.getByText(/^Each task's worktree/)).toBeTruthy();
  });

  test("a stream left as its count was stale counts again, and the new counts are sent", async () => {
    const again: LogPreview = { rows: 2, streams: [{ stream: "task-5", rows: 2, archived: false }] };
    vi.mocked(previewLogRedaction).mockResolvedValueOnce(PREVIEW).mockResolvedValueOnce(again);
    vi.mocked(redactLog)
      .mockResolvedValueOnce(PARTLY)
      .mockResolvedValueOnce({ rows: 2, streams: [{ stream: "task-5", at: AT, rows: 2 }], failures: [], warnings: [] });
    await count();
    await act(async () => fireEvent.click(button("Redact 5 rows")));
    await act(async () => fireEvent.click(button("Count again")));

    expect(previewLogRedaction).toHaveBeenCalledTimes(2);
    expect(within(row("task-5")[0]).getByText("2 rows")).toBeTruthy();
    await act(async () => fireEvent.click(button("Redact 2 rows")));
    expect(redactLog).toHaveBeenLastCalledWith(SECRET, "pasted everywhere", { "task-5": 2 });
    expect(screen.getByText(`Redacted 2 rows at ${dateTime(AT)}.`)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Count again" })).toBeNull();
  });

  test("a redaction refused as a whole counts again and says why", async () => {
    const again: LogPreview = { rows: 3, streams: [{ stream: "task-3", rows: 3, archived: false }] };
    vi.mocked(previewLogRedaction)
      .mockResolvedValueOnce({ rows: 2, streams: [{ stream: "task-3", rows: 2, archived: false }] })
      .mockResolvedValueOnce(again);
    vi.mocked(redactLog).mockRejectedValueOnce(new ApiError("task-3: 3 rows hold this text now, not 2: count them again", 409));
    await count();
    await act(async () => fireEvent.click(button("Redact 2 rows")));

    expect(redactLog).toHaveBeenCalledWith(SECRET, "pasted everywhere", { "task-3": 2 });
    expect(screen.getByText(/^3 rows hold this text\./)).toBeTruthy();
    expect(screen.getByText(/3 rows hold this text now, not 2/).textContent).toContain("so the rows were counted again");
    expect(button("Redact 3 rows")).toBeTruthy();
  });

  test("a preview refused shows the reason with the text kept", async () => {
    vi.mocked(previewLogRedaction).mockRejectedValue(new ApiError("no row of the log holds the secret", 409));
    await count();
    expect(screen.getByText("It can't be redacted: no row of the log holds the secret")).toBeTruthy();
    expect((screen.getByLabelText("The text to take out") as HTMLTextAreaElement).value).toBe(SECRET);
  });
});
