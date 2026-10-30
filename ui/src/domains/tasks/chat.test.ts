import { describe, expect, test } from "vitest";
import type { LogEvent, TaskDetail } from "@/api/types";
import { CHAT_TYPES, chatOf } from "./chat";
import catalogue from "../../../../internal/core/events/types.go?raw";

// Every type of event the log has, from the Go catalogue itself, so a new
// type is checked too.
const EVERY_TYPE = [...catalogue.matchAll(/\bType = "([^"]+)"/g)].map((match) => match[1]);

const task = { id: "task-1", coordinator: "tech-lead", turnList: [{ id: "turn-1", agent: "engineer" }] } as TaskDetail;

// eventOf is an event of a type with every field there is filled in, so
// chatOf shows something for it if it reads that type at all.
function eventOf(type: string): LogEvent {
  const data = new Proxy({}, { get: (_, key) => `some ${String(key)}` });
  return { position: 1, at: "2026-10-19T10:00:00Z", type, actor: "agent:tech-lead", correlation: "turn-1", data };
}

describe("the chat's types", () => {
  test("the catalogue is read", () => {
    expect(EVERY_TYPE).toContain("task.created");
    expect(EVERY_TYPE.length).toBeGreaterThan(50);
  });

  test("chatOf shows nothing for a type CHAT_TYPES lacks, so the chat never misses an event it would show", () => {
    const missed = EVERY_TYPE.filter((type) => !(CHAT_TYPES as readonly string[]).includes(type)).filter(
      (type) => chatOf(task, [eventOf(type)]).length > 0,
    );
    expect(missed).toEqual([]);
  });

  test("every type in CHAT_TYPES is in the log's catalogue, and chatOf shows it", () => {
    for (const type of CHAT_TYPES) {
      expect(EVERY_TYPE).toContain(type);
      expect(chatOf(task, [eventOf(type)]), type).not.toEqual([]);
    }
  });
});
