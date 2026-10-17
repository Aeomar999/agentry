import type { TaskSummary } from "@/api/types";

// Dragging cards within a column of cards, on the board and in Plan: where
// a card being moved would land, and the line that shows it.

// slotAt is where in a column a card being moved would land: the number of
// the other cards ([data-task]) above the pointer.
export function slotAt(column: HTMLElement, y: number, movingId: string): number {
  const cards = [...column.querySelectorAll<HTMLElement>("[data-task]")].filter((card) => card.dataset.task !== movingId);
  return cards.filter((card) => {
    const box = card.getBoundingClientRect();
    return box.top + box.height / 2 < y;
  }).length;
}

// slotIndex is the slot just above the card at index, or -1 above the card
// being moved, which has no slot of its own.
export function slotIndex(cards: TaskSummary[], movingId: string, index: number): number {
  if (cards[index].id === movingId) return -1;
  return cards.slice(0, index).filter((card) => card.id !== movingId).length;
}

export function DropLine() {
  return <div className="h-px shrink-0 bg-brand" aria-hidden />;
}
