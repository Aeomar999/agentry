import { PointerEvent as ReactPointerEvent, useRef } from "react";

// Dragging the top edge sets the height: up is taller.
export function ResizeHandle({ height, onResize }: { height: number; onResize: (height: number) => void }) {
  const start = useRef<{ y: number; height: number } | null>(null);

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    start.current = { y: event.clientY, height };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    if (start.current) onResize(start.current.height + start.current.y - event.clientY);
  }
  function onPointerUp() {
    start.current = null;
  }

  return (
    <div
      role="separator"
      aria-orientation="horizontal"
      aria-label="Drag to resize the terminal"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      className="absolute inset-x-0 -top-[3px] z-10 h-[6px] cursor-row-resize hover:bg-divider-2 active:bg-ink-faint"
    />
  );
}
