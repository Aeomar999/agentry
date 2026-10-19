import { describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { keepFocusInView } from "./focus-in-view";
import { layOut } from "@/test-layout";

layOut();

// A 400px scroller with a 44px band and 48px kept clear at the bottom, so
// focus belongs between 44 and 352, scrolled 100px down.
function feed() {
  render(
    <div
      data-testid="scroller"
      data-lines="0-400"
      style={{ scrollPaddingTop: "44px", scrollPaddingBottom: "48px" }}
      onFocus={keepFocusInView}
    >
      <a href="#under" data-lines="30-48">
        under
      </a>
      <a href="#inside" data-lines="200-218">
        inside
      </a>
      <a href="#below" data-lines="344-362">
        below
      </a>
      <button type="button" data-lines="300-700">
        taller
      </button>
    </div>,
  );
  const scroller = screen.getByTestId("scroller");
  scroller.scrollTop = 100;
  return scroller;
}

describe("keepFocusInView", () => {
  test("scrolls an element under the band down to just below it", () => {
    const scroller = feed();
    screen.getByText("under").focus();
    expect(scroller.scrollTop).toBe(86);
  });

  test("scrolls an element past the bottom up to just above the padding", () => {
    const scroller = feed();
    screen.getByText("below").focus();
    expect(scroller.scrollTop).toBe(110);
  });

  test("leaves an element already in view", () => {
    const scroller = feed();
    screen.getByText("inside").focus();
    expect(scroller.scrollTop).toBe(100);
  });

  test("brings an element taller than the room only up to the band", () => {
    const scroller = feed();
    screen.getByText("taller").focus();
    expect(scroller.scrollTop).toBe(356);
  });

  test("leaves an element whose lines shown above a fold's cut are in view", () => {
    const scroller = feed();
    render(
      <div data-fold-cut="100" data-lines="250-350">
        <pre tabIndex={0} data-lines="300-2300">
          folded code
        </pre>
      </div>,
      { container: scroller },
    );
    screen.getByText("folded code").focus();
    expect(scroller.scrollTop).toBe(100);
  });

  test("scrolls only far enough to show the lines above a fold's cut", () => {
    const scroller = feed();
    render(
      <div data-fold-cut="80" data-lines="300-380">
        <pre tabIndex={0} data-lines="330-2330">
          folded code
        </pre>
      </div>,
      { container: scroller },
    );
    screen.getByText("folded code").focus();
    expect(scroller.scrollTop).toBe(128);
  });

  test("doesn't scroll for a click", () => {
    const scroller = feed();
    const visible = vi.spyOn(Element.prototype, "matches").mockReturnValue(false);
    screen.getByText("below").focus();
    visible.mockRestore();
    expect(scroller.scrollTop).toBe(100);
  });
});
