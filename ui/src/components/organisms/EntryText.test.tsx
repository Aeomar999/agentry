import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { TaskSummary } from "@/api/types";
import { TaskTitlesProvider, useTaskLinks } from "@/live/refs";
import { layout, layOut } from "@/test-layout";
import { EntryText } from "./EntryText";

layOut();

// MonacoColorize stands in for Monaco: loading it is what the last test watches.
const loaded = vi.fn();
function monacoColorize() {
  loaded();
  return { default: ({ code }: { code: string }) => <>{code}</> };
}
vi.mock("@/components/molecules/MonacoColorize", () => monacoColorize());

const tasks = [{ id: "task-21", title: "Fix the chat links" }] as TaskSummary[];
const commits = ["3f2a9c1d0e4b5a6978877665544332211aabbccd"];

function Entry({ text }: { text: string }) {
  return <EntryText text={text} links={useTaskLinks("task-75", commits)} lines={10} kind="report" />;
}

function entries(...texts: string[]) {
  return render(
    <TaskTitlesProvider tasks={tasks}>
      <MemoryRouter initialEntries={["/tasks/task-75"]}>
        {texts.map((text, n) => (
          <Entry key={n} text={text} />
        ))}
      </MemoryRouter>
    </TaskTitlesProvider>,
  );
}

const REPORT = `## Done

Lint and \`go test ./...\` pass.

- one
- two

| Check | Result |
|---|---|
| lint | pass |

\`\`\`TS
const to = linkInto(task, href);
\`\`\``;

describe("EntryText", () => {
  test("renders the Markdown: headings, lists, inline code, a table and a fence in a code box", () => {
    const { container } = entries(REPORT);
    expect(screen.getByRole("heading", { level: 2, name: "Done" })).toBeTruthy();
    expect(screen.getAllByRole("listitem").map((item) => item.textContent)).toEqual(["one", "two"]);
    expect(container.querySelector("code")!.textContent).toBe("go test ./...");
    expect(screen.getByRole("table")).toBeTruthy();
    expect(screen.getByText("const to = linkInto(task, href);").closest("pre")).toBeTruthy();
    expect(screen.getByText("ts")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy code" })).toBeTruthy();
    expect(container.textContent).not.toMatch(/\*\*|\|---|```/);
  });

  test("opens an external link in a new tab, with the ↗ mark", () => {
    entries("see https://example.com/x");
    const link = screen.getByRole("link", { name: "https://example.com/x (opens in a new tab)" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(link.querySelector("svg")).toBeTruthy();
  });

  test("sends a repo path to the Workspace preview", () => {
    entries("[the design](docs/design.md)");
    expect(screen.getByRole("link", { name: "the design (opens in a new tab)" }).getAttribute("href")).toBe(
      "/tasks/task-75/workspace?file=docs%2Fdesign.md&view=preview",
    );
  });

  test("links a task id the board knows and a SHA of the task's commits, and nothing else", () => {
    entries("task-21 and task-999, `3f2a9c1` and `9e1d04b`");
    expect(screen.getByRole("link", { name: "task-21, Fix the chat links" }).getAttribute("href")).toBe("/tasks/task-21");
    expect(screen.getByRole("link", { name: "commit 3f2a9c1 (opens in a new tab)" }).getAttribute("href")).toBe(
      "/tasks/task-75/workspace?commit=3f2a9c1d0e4b5a6978877665544332211aabbccd",
    );
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });

  test("draws a javascript: link as its label", () => {
    const { container } = entries("[click me](javascript:alert(1)) or javascript:alert(2)");
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("click me");
  });

  test("keeps a code box's Copy and Show all out of the Tab order while the report's fold cuts it off", () => {
    // Twelve 20px paragraphs, a link on the second, then a 15-line fence at
    // 260px: the report folds at 10 lines (a cut at 200px), the fence at 12.
    const paragraphs = Array.from({ length: 12 }, (_, n) => (n === 1 ? "para 1, see https://example.com" : `para ${n}`));
    layout.lines = (element) => {
      const para = element.tagName === "P" && element.textContent!.match(/^para (\d+)/);
      if (para) return `${Number(para[1]) * 20}-${Number(para[1]) * 20 + 20}`;
      if (element.tagName === "PRE") return Array.from({ length: 15 }, (_, n) => `${260 + n * 20}-${280 + n * 20}`).join(" ");
      if (element.getAttribute("aria-label") === "Copy code") return "260-288";
      if (element.getAttribute("aria-label")?.endsWith("of this code")) return "560-580";
      return null;
    };
    const code = Array.from({ length: 15 }, (_, n) => `line ${n}`).join("\n");
    entries(`${paragraphs.join("\n\n")}\n\n\`\`\`go\n${code}\n\`\`\``);
    const tabOrder = () =>
      [...document.querySelectorAll<HTMLElement>("a[href], button")]
        .filter((element) => element.tabIndex >= 0)
        .map((element) => element.getAttribute("aria-label") ?? element.textContent);
    const report = screen.getByRole("button", { name: /of this report$/ });
    expect(tabOrder()).toEqual(["https://example.com (opens in a new tab)", report.getAttribute("aria-label")]);
    fireEvent.click(report);
    expect(tabOrder()).toEqual([
      "https://example.com (opens in a new tab)",
      "Copy code",
      "Show all 15 lines of this code",
      "Show less of this report",
    ]);
    fireEvent.click(report);
    expect(tabOrder()).toEqual(["https://example.com (opens in a new tab)", report.getAttribute("aria-label")]);
  });

  test("loads no Monaco for many entries with code while none is near the screen", async () => {
    vi.resetModules(); // so a load by an earlier test doesn't hide one here
    vi.doMock("@/components/molecules/MonacoColorize", monacoColorize);
    loaded.mockClear();
    const { EntryText } = await import("./EntryText");
    render(
      <>
        {Array.from({ length: 500 }, (_, n) => (
          <EntryText key={n} text={`Ran:\n\n\`\`\`bash\nnpm test # ${n}\n\`\`\``} links={{}} lines={10} kind="report" />
        ))}
      </>,
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(screen.getByText("npm test # 499")).toBeTruthy();
    expect(loaded).not.toHaveBeenCalled();
  });
});
