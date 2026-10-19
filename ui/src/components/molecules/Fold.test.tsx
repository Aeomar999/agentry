import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
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
});
