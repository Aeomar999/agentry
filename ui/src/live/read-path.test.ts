import { beforeEach, describe, expect, test, vi } from "vitest";
import { send } from "@/api/client";
import { readPath } from "./read-path";

vi.mock("@/api/client", () => ({ send: vi.fn() }));

// replyWith makes the next read answer {"ok": true} with these headers.
function replyWith(headers: Record<string, string>) {
  vi.mocked(send).mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { headers }));
}

describe("readPath", () => {
  beforeEach(() => vi.mocked(send).mockReset());

  test("reads the path and the log position from the header", async () => {
    replyWith({ "X-Agentry-Position": "18240" });
    expect(await readPath("/api/tasks")).toEqual({ data: { ok: true }, position: 18240 });
    expect(send).toHaveBeenCalledWith("/api/tasks", { method: "GET" });
  });

  test("gives no position when the header is missing", async () => {
    replyWith({});
    expect(await readPath("/api/tasks")).toEqual({ data: { ok: true }, position: undefined });
  });

  test.each(["", "abc", "12abc", "-5", "1e3", "4.5"])("gives no position for the header %j", async (header) => {
    replyWith({ "X-Agentry-Position": header });
    expect((await readPath("/api/tasks")).position).toBeUndefined();
  });
});
