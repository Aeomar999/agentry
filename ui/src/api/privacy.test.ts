import { beforeEach, describe, expect, test, vi } from "vitest";
import { api } from "./client";
import { eraseTask, previewAgentsRedaction, previewLogRedaction, previewRedaction, redact, redactAgents, redactLog } from "./privacy";

vi.mock("./client", () => ({ api: vi.fn() }));

// What each call posts, and where: the server holds a redaction to the
// rows its preview counted only when the page sends them.
describe("privacy", () => {
  beforeEach(() => {
    vi.mocked(api).mockReset();
    vi.mocked(api).mockResolvedValue({ rows: 2 });
  });

  test("erases a task, with why", async () => {
    await eraseTask("task-1", "w");
    expect(api).toHaveBeenCalledWith("POST", "/api/tasks/task-1/erase", { why: "w" });
  });

  test("previews a task's redaction, and redacts the rows counted", async () => {
    expect(await previewRedaction("task-1", "S")).toBe(2);
    expect(api).toHaveBeenLastCalledWith("POST", "/api/tasks/task-1/redact/preview", { text: "S" });
    await redact("task-1", "S", "w", 2);
    expect(api).toHaveBeenLastCalledWith("POST", "/api/tasks/task-1/redact", { text: "S", why: "w", rows: 2 });
  });

  test("previews the agents' redaction, and redacts the rows counted", async () => {
    expect(await previewAgentsRedaction("S")).toBe(2);
    expect(api).toHaveBeenLastCalledWith("POST", "/api/agents/redact/preview", { text: "S" });
    await redactAgents("S", "w", 2);
    expect(api).toHaveBeenLastCalledWith("POST", "/api/agents/redact", { text: "S", why: "w", rows: 2 });
  });

  test("previews a redaction across the log, and redacts the rows counted in each stream", async () => {
    await previewLogRedaction("S");
    expect(api).toHaveBeenLastCalledWith("POST", "/api/redact/preview", { text: "S" });
    await redactLog("S", "w", { "task-3": 2, agents: 1 });
    expect(api).toHaveBeenLastCalledWith("POST", "/api/redact", { text: "S", why: "w", counts: { "task-3": 2, agents: 1 } });
  });
});
