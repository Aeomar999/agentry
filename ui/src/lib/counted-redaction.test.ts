import { describe, expect, test, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { useCountedRedaction } from "./counted-redaction";

// A count can be any shape: here a preview's rows in each stream.
type Count = { rows: Record<string, number> };

function counted(previews: Count[], redact: (text: string, why: string, count: Count) => Promise<string>) {
  const preview = vi.fn(async () => previews.shift()!);
  const onRedacted = vi.fn();
  const { result } = renderHook(() => useCountedRedaction(preview, redact, onRedacted));
  return { result, preview, onRedacted };
}

describe("useCountedRedaction", () => {
  test("sends the count confirmed and says what was done", async () => {
    const redact = vi.fn(async () => "done");
    const { result, onRedacted } = counted([{ rows: { "task-1": 2 } }], redact);
    await act(() => result.current.count("secret"));
    expect(result.current.stage).toEqual({ step: "confirm", count: { rows: { "task-1": 2 } }, note: "" });
    await act(() => result.current.confirm("secret", "leaked", { rows: { "task-1": 2 } }));
    expect(redact).toHaveBeenCalledWith("secret", "leaked", { rows: { "task-1": 2 } });
    expect(result.current.stage).toEqual({ step: "done", redaction: "done" });
    expect(onRedacted).toHaveBeenCalledOnce();
  });

  test("counts again when the server refuses the count 409", async () => {
    const redact = vi.fn(async () => {
      throw new ApiError("the rows changed", 409);
    });
    const { result, preview, onRedacted } = counted([{ rows: { "task-1": 2 } }, { rows: { "task-1": 3 } }], redact);
    await act(() => result.current.count("secret"));
    await act(() => result.current.confirm("secret", "", { rows: { "task-1": 2 } }));
    expect(preview).toHaveBeenCalledTimes(2);
    expect(result.current.stage).toEqual({
      step: "confirm",
      count: { rows: { "task-1": 3 } },
      note: "Agentry refused it (the rows changed), so the rows were counted again.",
    });
    expect(onRedacted).not.toHaveBeenCalled();
  });

  test("keeps the count it had when the redaction fails otherwise", async () => {
    const redact = vi.fn(async () => {
      throw new ApiError("the disk is full", 500);
    });
    const { result, preview } = counted([{ rows: { "task-1": 2 } }], redact);
    await act(() => result.current.count("secret"));
    await act(() => result.current.confirm("secret", "", { rows: { "task-1": 2 } }));
    expect(preview).toHaveBeenCalledOnce();
    expect(result.current.stage).toEqual({
      step: "confirm",
      count: { rows: { "task-1": 2 } },
      note: "It wasn't redacted: the disk is full",
    });
  });

  test("goes back to marking with the refusal when the count fails", async () => {
    const preview = vi.fn(async (): Promise<Count> => {
      throw new ApiError("the text is too short", 400);
    });
    const { result } = renderHook(() => useCountedRedaction(preview, vi.fn()));
    await act(() => result.current.count("ab"));
    expect(result.current.stage).toEqual({ step: "mark", refusal: "the text is too short" });
  });
});
