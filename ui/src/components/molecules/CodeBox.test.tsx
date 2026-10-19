import { beforeEach, describe, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { intersect, layOut } from "@/test-layout";

layOut();

// MonacoColorize stands in for Monaco: loading it is what the tests watch.
const loaded = vi.fn();

async function codeBox(colorize: () => object = colorizer) {
  vi.resetModules();
  loaded.mockClear();
  vi.doMock("./MonacoColorize", () => {
    loaded();
    return colorize();
  });
  return (await import("./CodeBox")).CodeBox;
}

function colorizer() {
  return {
    default: ({ code, language }: { code: string; language: string }) => (
      <span data-testid="coloured" data-language={language}>
        {code}
      </span>
    ),
  };
}

beforeEach(() => vi.spyOn(console, "warn").mockImplementation(() => {}));

describe("CodeBox", () => {
  test("loads no Monaco while no code box is near the screen", async () => {
    const CodeBox = await codeBox();
    render(
      <>
        {Array.from({ length: 500 }, (_, n) => (
          <CodeBox key={n} code={`go test ./... # ${n}`} language="bash" command />
        ))}
      </>,
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(loaded).not.toHaveBeenCalled();
    expect(screen.getByText("go test ./... # 499")).toBeTruthy();
  });

  test("colours a box once it comes near, with its language as written", async () => {
    const CodeBox = await codeBox();
    const { container } = render(
      <>
        <CodeBox code="ls" language="bash" command />
        <CodeBox code="const x = 1;" language="ts" />
      </>,
    );
    intersect(container.querySelectorAll("pre > span")[1]);
    const coloured = await screen.findByTestId("coloured");
    expect(coloured.textContent).toBe("const x = 1;");
    expect(coloured.getAttribute("data-language")).toBe("ts");
    expect(screen.getAllByTestId("coloured")).toHaveLength(1);
    expect(loaded).toHaveBeenCalledTimes(1);
  });

  test("keeps the plain text, with nothing said, when Monaco fails to load", async () => {
    const CodeBox = await codeBox(() => {
      throw new Error("chunk failed");
    });
    const { container } = render(<CodeBox code="go vet ./..." language="go" />);
    intersect(container.querySelector("pre > span")!);
    await waitFor(() => expect(console.warn).toHaveBeenCalledTimes(1));
    expect(container.querySelector("pre")!.textContent).toBe("go vet ./...");
    expect(screen.queryByTestId("coloured")).toBeNull();
    expect(container.textContent).not.toMatch(/error|failed/i);
  });

  test("labels a fence with its language, lowercased, and gives it Copy", async () => {
    const CodeBox = await codeBox();
    render(<CodeBox code="SELECT 1;" language="SQL" />);
    expect(screen.getByText("sql")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy code" })).toBeTruthy();
  });

  test("shows no label for a fence with no language", async () => {
    const CodeBox = await codeBox();
    const { container } = render(<CodeBox code="plain" language="" />);
    expect(container.querySelector(".font-mono.truncate")).toBeNull();
  });

  test("gives a command no Copy and no label", async () => {
    const CodeBox = await codeBox();
    render(<CodeBox code="npm test" language="bash" command />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByText("bash")).toBeNull();
  });
});
