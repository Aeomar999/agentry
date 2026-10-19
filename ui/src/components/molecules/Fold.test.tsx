import { describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { Fold } from "./Fold";
import { layout, layOut } from "@/test-layout";

layOut();

// Ten 20px lines, folded at 4 (a cut at 80px), with links on lines 1, 4 and
// 6, one that wraps from line 4 to 5, and two with tabindex of their own.
const TEN_LINES = Array.from({ length: 10 }, (_, n) => `${n * 20}-${n * 20 + 20}`).join(" ");

function report() {
  return render(reportFold());
}

function reportFold() {
  return (
    <Fold lines={4} lineHeight={20} kind="report">
      <p data-lines={TEN_LINES}>
        A long report.
        <a href="#first" data-lines="0-20">
          first
        </a>
        <a href="#fourth" data-lines="60-80">
          fourth
        </a>
        <a href="#wraps" data-lines="60-80 80-100">
          wraps
        </a>
        <a href="#sixth" data-lines="100-120">
          sixth
        </a>
        <a href="#own" tabIndex={0} data-lines="120-140">
          own
        </a>
        <span tabIndex={-1} data-lines="140-160">
          skipped
        </span>
      </p>
    </Fold>
  );
}

// tabOrder is what Tab reaches, in order: what can take focus and isn't taken out.
function tabOrder(): string[] {
  return [...document.querySelectorAll<HTMLElement>("a[href], button, [tabindex]")]
    .filter((element) => element.tabIndex >= 0)
    .map((element) => element.getAttribute("aria-label") ?? element.textContent!.trim());
}

describe("Fold", () => {
  test("folded, Tab reaches only the links above the cut, then Show all", () => {
    report();
    expect(tabOrder()).toEqual(["first", "fourth", "wraps", "Show all 10 lines of this report"]);
  });

  test("names the button as copy.md says, and controls the text", () => {
    const { container } = report();
    const button = screen.getByRole("button");
    expect(button.textContent).toBe("Show all · 10 lines");
    expect(button.getAttribute("aria-label")).toBe("Show all 10 lines of this report");
    expect(button.getAttribute("aria-expanded")).toBe("false");
    const text = container.querySelector(`[id="${button.getAttribute("aria-controls")}"]`) as HTMLElement;
    expect(text.textContent).toContain("A long report.");
    expect(text.style.maxHeight).toBe("80px");
  });

  test("Show all opens every link, and Show less only the visible ones again", () => {
    report();
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByRole("button").textContent).toBe("Show less");
    expect(screen.getByRole("button").getAttribute("aria-label")).toBe("Show less of this report");
    expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe("true");
    expect(tabOrder()).toEqual(["first", "fourth", "wraps", "sixth", "own", "Show less of this report"]);
    fireEvent.click(screen.getByRole("button"));
    expect(tabOrder()).toEqual(["first", "fourth", "wraps", "Show all 10 lines of this report"]);
  });

  test("keeps the tabindex an element had and gives it back", () => {
    const { container } = report();
    const own = screen.getByText("own");
    const skipped = screen.getByText("skipped");
    const sixth = screen.getByText("sixth");
    expect(own.getAttribute("data-fold-tabindex")).toBe("0");
    expect(skipped.getAttribute("data-fold-tabindex")).toBe("-1");
    expect(sixth.getAttribute("data-fold-tabindex")).toBe("");
    fireEvent.click(screen.getByRole("button"));
    expect(own.getAttribute("tabindex")).toBe("0");
    expect(skipped.getAttribute("tabindex")).toBe("-1");
    expect(sixth.hasAttribute("tabindex")).toBe(false);
    expect(container.querySelector("[data-fold-tabindex]")).toBeNull();
  });

  test("positions the clipping box, so text placed absolutely in it is clipped with it", () => {
    const { container } = report();
    expect(container.querySelector("[data-fold-cut]")!.className.split(" ")).toContain("relative");
  });

  test("takes a cut-off box that scrolls out of the Tab order, and gives it back", () => {
    render(
      <Fold lines={4} lineHeight={20} kind="report">
        <p data-lines="0-20 20-40 40-60 60-80 80-100">A long report.</p>
        <pre style={{ overflowX: "auto" }} data-lines="100-120">
          wide code
        </pre>
      </Fold>,
    );
    const code = screen.getByText("wide code");
    expect(code.getAttribute("tabindex")).toBe("-1");
    fireEvent.click(screen.getByRole("button"));
    expect(code.hasAttribute("tabindex")).toBe(false);
  });

  test("looks up the style only of a box given an overflow", () => {
    const styles = vi.spyOn(window, "getComputedStyle");
    render(
      <Fold lines={4} lineHeight={20} kind="report">
        <p data-lines={TEN_LINES}>
          A long report. <a href="#link">link</a>
        </p>
        <pre className="overflow-x-auto">code</pre>
      </Fold>,
    );
    const looked = styles.mock.calls.map(([element]) => element.tagName);
    styles.mockRestore();
    expect(looked.length).toBeGreaterThan(0);
    expect(looked.every((tag) => tag === "PRE")).toBe(true);
  });

  test("doesn't redo the Tab order on a render that changes nothing", () => {
    const { rerender } = report();
    const reads = vi.spyOn(Element.prototype, "getClientRects");
    rerender(reportFold());
    rerender(reportFold());
    expect(reads).not.toHaveBeenCalled();
    reads.mockRestore();
  });

  test("redoes the Tab order once the render after a remeasure puts the new cut on the box", async () => {
    // Lines of 20px cut at 80; then lines of 24px, cut at 72, with a link (no
    // text, so no line of the cut's own) ending at 78: under the old cut, so
    // a pass before the render keeps it.
    let grow = () => {};
    function Growing() {
      const [tall, setTall] = useState(false);
      grow = () => setTall(true);
      return tall ? (
        <p key="tall" data-lines="0-24 24-48 48-72 72-96 96-120">
          Taller lines. <a href="#late" aria-label="late" data-lines="60-78" />
        </p>
      ) : (
        <p key="short" data-lines={TEN_LINES}>
          A long report. <a href="#late" aria-label="late" data-lines="60-78" />
        </p>
      );
    }
    const { container } = render(
      <Fold lines={4} lineHeight={20} kind="report">
        <Growing />
      </Fold>,
    );
    expect(tabOrder()).toEqual(["late", "Show all 10 lines of this report"]);
    act(() => grow());
    await waitFor(() => expect(container.querySelector("[data-fold-cut]")!.getAttribute("data-fold-cut")).toBe("72"));
    expect(tabOrder()).toEqual(["Show all 6 lines of this report"]);
  });

  test("redoes the Tab order when the text moves a link below the cut without changing its size", async () => {
    let moveDown = () => {};
    function Moving() {
      const [lines, setLines] = useState("0-20");
      moveDown = () => setLines("100-120");
      return (
        <a key={lines} href="#moving" data-lines={lines}>
          moving
        </a>
      );
    }
    render(
      <Fold lines={4} lineHeight={20} kind="report">
        <p data-lines={TEN_LINES}>
          A long report. <Moving />
        </p>
      </Fold>,
    );
    expect(tabOrder()).toEqual(["moving", "Show all 10 lines of this report"]);
    act(() => moveDown());
    await waitFor(() => expect(tabOrder()).toEqual(["Show all 10 lines of this report"]));
  });

  test("never uses inert", () => {
    const { container } = report();
    expect(container.querySelector("[inert]")).toBeNull();
    fireEvent.click(screen.getByRole("button"));
    expect(container.querySelector("[inert]")).toBeNull();
  });

  test("cuts at the bottom of the last whole line above the limit", () => {
    const { container } = render(
      <Fold lines={4} lineHeight={20} kind="brief">
        <p data-lines="0-24 24-48 48-72 72-96 96-120">Lines of 24px.</p>
      </Fold>,
    );
    expect(container.querySelector<HTMLElement>("[data-fold-cut]")!.style.maxHeight).toBe("72px");
  });

  test("shows short text whole, with no button", () => {
    const { container } = render(
      <Fold lines={4} lineHeight={20} kind="note">
        <p data-lines="0-20 20-40 40-60">Three lines.</p>
      </Fold>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(container.querySelector("[data-fold-cut]")).toBeNull();
  });

  test("measures once, not again on a render that changes nothing", () => {
    const measures = vi.spyOn(document, "createRange");
    const { rerender } = report();
    const once = measures.mock.calls.length;
    rerender(reportFold());
    rerender(reportFold());
    expect(once).toBeGreaterThan(0);
    expect(measures.mock.calls.length).toBe(once);
    measures.mockRestore();
  });

  test("measures again when the text changes", async () => {
    const { rerender } = render(
      <Fold lines={4} lineHeight={20} kind="note">
        <p data-lines="0-20">One line.</p>
      </Fold>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    rerender(
      <Fold lines={4} lineHeight={20} kind="note">
        <p data-lines={TEN_LINES}>Ten lines now.</p>
      </Fold>,
    );
    expect((await screen.findByRole("button")).textContent).toBe("Show all · 10 lines");
  });

  test("Show less scrolls back only when the entry's top went above the band", () => {
    report();
    const button = screen.getByRole("button");
    fireEvent.click(button);
    fireEvent.click(button);
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
    fireEvent.click(button);
    layout.shift = 150; // scrolled down, past the entry's top
    fireEvent.click(button);
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  // A report folded at 4 lines (cut at 80px) with a framed fold, a code box, inside it.
  function reportWithBox(box: string) {
    return render(
      <Fold lines={4} lineHeight={20} kind="report">
        <p data-lines="0-20 20-40">Two lines of report.</p>
        <Fold lines={12} lineHeight={20} kind="code" className="frame">
          <pre data-lines={box}>code</pre>
        </Fold>
        <p data-lines="200-220">After the box.</p>
      </Fold>,
    );
  }

  test("a cut through a code box draws a hairline at the cut, a pixel below it", () => {
    const { container } = reportWithBox("40-60 60-80 80-100 100-120");
    const outer = container.querySelector<HTMLElement>("[data-fold-cut]")!;
    expect(outer.className).toContain("border-b border-divider");
    expect(outer.style.maxHeight).toBe("81px");
  });

  test("a cut above or below a code box draws none", () => {
    for (const box of ["100-120 120-140", "20-40 40-60"]) {
      const { container, unmount } = reportWithBox(box);
      const outer = container.querySelector<HTMLElement>("[data-fold-cut]")!;
      expect(outer.className).not.toContain("border-b");
      unmount();
    }
  });

  test("a cut through plain text draws none", () => {
    const { container } = report();
    expect(container.querySelector<HTMLElement>("[data-fold-cut]")!.className).not.toContain("border-b");
  });
});
