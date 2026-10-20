import { beforeEach, describe, expect, test, vi } from "vitest";
import { api, ApiError, send } from "@/api/client";
import type { TeamTemplate } from "@/api/types";
import {
  checkTemplateName,
  deleteTemplate,
  importTemplate,
  saveAsTemplate,
  templateExportURL,
  templateGroups,
  whyTemplateNotDeleted,
  whyTemplateNotImported,
  whyTemplateNotSaved,
} from "./templates";

// The client reads the address when it loads, and there is none here.
vi.mock("@/api/client", () => ({
  api: vi.fn(),
  send: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(message: string, status: number) {
      super(message);
      this.status = status;
    }
  },
}));
vi.mock("@/live/use-resource", () => ({ useResource: vi.fn() }));

const template = (id: string, mine?: { savedAt: string; from?: string }): TeamTemplate => ({
  id,
  team: id,
  description: "",
  features: [],
  agents: [],
  userMade: mine !== undefined,
  ...mine,
});

describe("checkTemplateName", () => {
  test("a blank name asks for one, in the copy deck's words", () => {
    expect(checkTemplateName("")).toBe("Give the template a name.");
    expect(checkTemplateName("   ")).toBe("Give the template a name.");
  });

  test("any other name is the server's to check", () => {
    expect(checkTemplateName(" Reading Room ")).toBeUndefined();
  });
});

describe("saveAsTemplate", () => {
  beforeEach(() => vi.mocked(api).mockReset());

  test("posts the name and what it's for, trimmed, to the team's route", async () => {
    vi.mocked(api).mockResolvedValueOnce(template("reading-room", { savedAt: "2026-10-12T09:30:00Z", from: "R&D 2" }));
    const saved = await saveAsTemplate("R&D 2", { name: "  Reading Room ", description: " Reads papers. " });
    expect(api).toHaveBeenCalledWith("POST", "/api/teams/R%26D%202/template", { name: "Reading Room", description: "Reads papers." });
    expect(saved.from).toBe("R&D 2");
  });
});

describe("whyTemplateNotSaved", () => {
  test.each(["You have a template called Papers already. Pick another name.", "Software is a built-in template. Pick another name."])(
    "a 409 is a name taken, said under the name: %s",
    (words) => {
      expect(whyTemplateNotSaved(new ApiError(words, 409))).toEqual({ kind: "name", message: words });
    },
  );

  test("any other refusal is a failure in the server's words", () => {
    const refusal = new ApiError("agent lead: a template's agents name no workers", 400);
    expect(whyTemplateNotSaved(refusal)).toEqual({
      kind: "failed",
      title: "Couldn't save the template",
      detail: "agent lead: a template's agents name no workers",
    });
  });

  test("an unreachable server says so, not fetch's words", () => {
    expect(whyTemplateNotSaved(new TypeError("Failed to fetch"))).toEqual({
      kind: "failed",
      title: "Couldn't save the template",
      detail: "Couldn't reach Agentry. Check it's running, then try again.",
    });
  });
});

describe("deleteTemplate", () => {
  beforeEach(() => vi.mocked(api).mockReset());

  test("deletes the template by its id", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    await deleteTemplate("reading-room");
    expect(api).toHaveBeenCalledWith("DELETE", "/api/teams/templates/reading-room");
  });

  test("a refusal, a 409 too, is a failure in the server's words", () => {
    const refusal = new ApiError("the research template comes with Agentry, so it can't be removed", 409);
    expect(whyTemplateNotDeleted(refusal)).toEqual({
      kind: "failed",
      title: "Couldn't delete the template",
      detail: "the research template comes with Agentry, so it can't be removed",
    });
  });
});

describe("export and import", () => {
  beforeEach(() => vi.mocked(send).mockReset());

  test("a template exports from its own route, as a link", () => {
    expect(templateExportURL("reading-room")).toBe("/api/teams/templates/reading-room/export");
  });

  test("a file imports as it is, and the reply is the template saved", async () => {
    const saved = template("reading-room", { savedAt: "2026-10-12T09:30:00Z" });
    vi.mocked(send).mockResolvedValueOnce(new Response(JSON.stringify(saved)));
    const file = new File(['{"id":"reading-room"}'], "reading-room.json", { type: "application/json" });

    expect(await importTemplate(file)).toEqual(saved);
    expect(send).toHaveBeenCalledWith("/api/teams/templates/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: file,
    });
  });

  test("a refused import is a failure in the server's words", () => {
    const refusal = new ApiError("a template has exactly one coordinator", 400);
    expect(whyTemplateNotImported(refusal)).toEqual({
      kind: "failed",
      title: "Couldn't import the template",
      detail: "a template has exactly one coordinator",
    });
  });
});

describe("templateGroups", () => {
  test("yours newest first, then the built-in ones as listed", () => {
    const list = [
      template("software"),
      template("research"),
      template("papers", { savedAt: "2026-10-01T09:00:00Z", from: "Papers" }),
      template("orders", { savedAt: "2026-10-12T09:00:00Z" }),
      template("desk", { savedAt: "2025-12-31T23:59:59Z" }),
    ];
    const { yours, builtIn } = templateGroups(list);
    expect(yours.map((t) => t.id)).toEqual(["orders", "papers", "desk"]);
    expect(builtIn.map((t) => t.id)).toEqual(["software", "research"]);
  });

  test("of two saved in the same second, the one listed later is newer", () => {
    const at = "2026-10-12T09:00:00Z";
    const { yours } = templateGroups([template("first", { savedAt: at }), template("second", { savedAt: at })]);
    expect(yours.map((t) => t.id)).toEqual(["second", "first"]);
  });

  test("with none of yours, yours is empty", () => {
    expect(templateGroups([template("software")])).toEqual({ yours: [], builtIn: [template("software")] });
  });
});
