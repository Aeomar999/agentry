import { beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { previewRedaction, redact } from "@/api/privacy";
import { dateTime } from "@/lib/time";
import { RedactText } from "./RedactText";

vi.mock("@/api/privacy", () => ({ eraseTask: vi.fn(), previewRedaction: vi.fn(), redact: vi.fn() }));

const SECRET = "sk-live-0123456789";
const AT = "2026-10-20T09:30:00Z";
// The server's warning while an agent's turn that started before the
// redaction is still running (internal/web redact_api.go stillRunning): a
// task's redaction answers its warnings in the same list.
const STILL_RUNNING =
  "an agent whose turn is running wrote its knowledge file, secret and all, as the turn started: its end redacts it again, unless Agentry restarts first, so if it does, redact again after any turn that was running ends";

let onRedacted = vi.fn();
let onClose = vi.fn();

beforeEach(() => {
  vi.mocked(previewRedaction).mockReset();
  vi.mocked(redact).mockReset();
  onRedacted = vi.fn();
  onClose = vi.fn();
});

function ask(marked = "") {
  render(<RedactText task="task-1" marked={marked} onClose={onClose} onRedacted={onRedacted} />);
}

const button = (name: string) => screen.getByRole("button", { name });
const field = () => screen.getByLabelText("The text to take out") as HTMLTextAreaElement;

async function count(text = SECRET) {
  fireEvent.change(field(), { target: { value: text } });
  await act(async () => fireEvent.click(button("Count the rows")));
}

describe("RedactText", () => {
  test("starts from the text marked on the page", () => {
    ask(SECRET);
    expect(field().value).toBe(SECRET);
  });

  test("shows the count first, redacts only once you say yes, then what it did and the note on backups", async () => {
    vi.mocked(previewRedaction).mockResolvedValue(3);
    vi.mocked(redact).mockResolvedValue({ stream: "task-1", at: AT, rows: 3, warnings: [] });
    ask();
    fireEvent.change(screen.getByLabelText("Why, for the log (optional)"), { target: { value: "pasted a key" } });
    await count();

    expect(previewRedaction).toHaveBeenCalledWith("task-1", SECRET);
    expect(screen.getByText(/^3 rows of task-1 hold this text\./).textContent).toContain("Nothing undoes this.");
    expect(redact).not.toHaveBeenCalled();

    await act(async () => fireEvent.click(button("Redact 3 rows")));
    expect(redact).toHaveBeenCalledWith("task-1", SECRET, "pasted a key");
    expect(onRedacted).toHaveBeenCalled();
    expect(screen.getByText(`Redacted 3 rows of task-1 at ${dateTime(AT)}.`)).toBeTruthy();
    expect(screen.getByText(/^Backups of agentry.db/).textContent).toContain(`made before ${dateTime(AT)} still hold the text`);
    expect(screen.queryByText("Some of it may be left for now")).toBeNull();
  });

  test("shows a refusal in the server's words, and redacts nothing", async () => {
    vi.mocked(previewRedaction).mockRejectedValue(new ApiError("the secret is a number in the JSON: mark the text around it too", 409));
    ask();
    await count("42");
    expect(screen.getByText("It can't be redacted: the secret is a number in the JSON: mark the text around it too")).toBeTruthy();
    expect(field().value).toBe("42");
    expect(redact).not.toHaveBeenCalled();
  });

  test("a redaction refused 409 counts the rows again and asks again with the new count", async () => {
    vi.mocked(previewRedaction).mockResolvedValueOnce(2).mockResolvedValueOnce(4);
    vi.mocked(redact)
      .mockRejectedValueOnce(new ApiError("task-1 took an event since its rows were read: find its rows again", 409))
      .mockResolvedValueOnce({ stream: "task-1", at: AT, rows: 4, warnings: [] });
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 2 rows")));

    expect(previewRedaction).toHaveBeenCalledTimes(2);
    expect(screen.getByText(/^4 rows of task-1 hold this text\./)).toBeTruthy();
    expect(screen.getByText(/took an event since its rows were read/).textContent).toContain("so the rows were counted again");
    expect(onRedacted).not.toHaveBeenCalled();

    await act(async () => fireEvent.click(button("Redact 4 rows")));
    expect(screen.getByText(`Redacted 4 rows of task-1 at ${dateTime(AT)}.`)).toBeTruthy();
  });

  test("a 409 whose new count is refused goes back to the text, with the reason", async () => {
    vi.mocked(previewRedaction).mockResolvedValueOnce(1).mockRejectedValueOnce(new ApiError("no row of task-1 holds the secret", 409));
    vi.mocked(redact).mockRejectedValueOnce(new ApiError("task-1 took an event since its rows were read: find its rows again", 409));
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 1 row")));
    expect(screen.getByText("It can't be redacted: no row of task-1 holds the secret")).toBeTruthy();
    expect(field().value).toBe(SECRET);
  });

  test("an error that isn't a refusal keeps the count and says it wasn't redacted", async () => {
    vi.mocked(previewRedaction).mockResolvedValue(1);
    vi.mocked(redact).mockRejectedValueOnce(new ApiError("database is locked", 500));
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 1 row")));
    expect(previewRedaction).toHaveBeenCalledTimes(1);
    expect(screen.getByText("It wasn't redacted: database is locked")).toBeTruthy();
    expect(button("Redact 1 row")).toBeTruthy();
  });

  test("shows the warnings the redaction gave, such as a turn still running", async () => {
    vi.mocked(previewRedaction).mockResolvedValue(1);
    vi.mocked(redact).mockResolvedValue({ stream: "task-1", at: AT, rows: 1, warnings: [STILL_RUNNING] });
    ask();
    await count();
    await act(async () => fireEvent.click(button("Redact 1 row")));
    expect(screen.getByText("Some of it may be left for now")).toBeTruthy();
    expect(screen.getByText(STILL_RUNNING)).toBeTruthy();
  });

  test("won't count a blank text", () => {
    ask("   ");
    expect((button("Count the rows") as HTMLButtonElement).disabled).toBe(true);
  });
});
