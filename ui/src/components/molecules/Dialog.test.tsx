import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { Dialog } from "./Dialog";

const scrim = () => screen.getByRole("dialog").parentElement!;

const inside = (onClose: () => void, held?: boolean) => (
  <Dialog title="Add a team" onClose={onClose} held={held}>
    <button type="button">Inside</button>
  </Dialog>
);

function dialog(held?: boolean) {
  const onClose = vi.fn();
  const { rerender } = render(inside(onClose, held));
  return { onClose, rerender: (held?: boolean) => rerender(inside(onClose, held)) };
}

describe("Dialog", () => {
  test("Esc or a mousedown outside closes it; one inside doesn't", () => {
    const { onClose } = dialog();
    fireEvent.mouseDown(screen.getByRole("button", { name: "Inside" }));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.mouseDown(scrim());
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  test("held, neither Esc nor a mousedown outside closes it, and once let go both do again", () => {
    const { onClose, rerender } = dialog(true);
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.mouseDown(scrim());
    expect(onClose).not.toHaveBeenCalled();

    rerender(false);
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.mouseDown(scrim());
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
