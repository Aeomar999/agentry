import { DragEvent, Fragment, ReactNode, useState } from "react";
import { cn } from "@/lib/cn";

export interface Column<Row> {
  header: string;
  render: (row: Row) => ReactNode;
  className?: string; // width, alignment; applied to the header and every cell
}

interface ResourceTableProps<Row> {
  columns: Column<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onOpen?: (row: Row) => void;
  activeKey?: string; // the row the keyboard is on
  empty?: ReactNode;
  // Rows can be dragged into a new order: called with the row and where it
  // lands, counted among the other rows.
  onReorder?: (row: Row, at: number) => void;
}

// Dense rows (36px), hairline separated, the active row a flat lighter
// surface. Rows open on click or Enter. Cells don't wrap; the one wide
// column (w-full max-w-0) truncates or clamps its own text. With onReorder,
// rows drag up and down, with a 1px line where one would land.
export function ResourceTable<Row>({ columns, rows, rowKey, onOpen, activeKey, empty, onReorder }: ResourceTableProps<Row>) {
  const [moving, setMoving] = useState<string | null>(null);
  const [slot, setSlot] = useState<number | null>(null);
  if (rows.length === 0 && empty) return <>{empty}</>;

  const others = rows.filter((row) => rowKey(row) !== moving);
  function onDragOver(event: DragEvent<HTMLTableSectionElement>) {
    if (moving === null) return;
    event.preventDefault();
    const above = [...event.currentTarget.querySelectorAll<HTMLElement>("tr[data-row]")]
      .filter((tr) => tr.dataset.row !== moving)
      .filter((tr) => {
        const box = tr.getBoundingClientRect();
        return box.top + box.height / 2 < event.clientY;
      });
    setSlot(above.length);
  }
  function onDrop(event: DragEvent) {
    event.preventDefault();
    const row = rows.find((r) => rowKey(r) === moving);
    if (row && slot !== null) onReorder?.(row, slot);
    setMoving(null);
    setSlot(null);
  }
  const line = (
    <tr aria-hidden>
      <td colSpan={columns.length} className="h-px bg-brand p-0" />
    </tr>
  );

  return (
    <div className="scrollbar-thin overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="h-8 border-b border-divider bg-surface-1">
            {columns.map((column) => (
              <th
                key={column.header}
                className={cn(
                  "px-3 text-left text-[11px] font-semibold tracking-wider whitespace-nowrap text-ink-muted uppercase first:pl-4",
                  column.className,
                )}
              >
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody onDragOver={onReorder ? onDragOver : undefined} onDrop={onReorder ? onDrop : undefined}>
          {rows.map((row) => {
            const key = rowKey(row);
            const active = key === activeKey;
            return (
              <Fragment key={key}>
                {slot !== null && key !== moving && others.indexOf(row) === slot && line}
                <tr
                  data-row={key}
                  tabIndex={onOpen ? 0 : undefined}
                  draggable={onReorder !== undefined}
                  onDragStart={(event) => {
                    event.dataTransfer.effectAllowed = "move";
                    event.dataTransfer.setData("text/plain", key);
                    setMoving(key);
                  }}
                  onDragEnd={() => {
                    setMoving(null);
                    setSlot(null);
                  }}
                  onClick={() => onOpen?.(row)}
                  onKeyDown={(event) => event.key === "Enter" && onOpen?.(row)}
                  data-active={active || undefined}
                  className={cn(
                    // The focus line goes inside, where the table's scroller can't clip it.
                    "h-row-list border-b border-divider transition-colors duration-(--dur-fast) focus-visible:-outline-offset-2",
                    onOpen && "cursor-pointer hover:bg-surface-2",
                    active && "bg-surface-3",
                    key === moving && "opacity-40",
                  )}
                >
                  {columns.map((column) => (
                    <td key={column.header} className={cn("px-3 py-1.5 align-middle whitespace-nowrap first:pl-4", column.className)}>
                      {column.render(row)}
                    </td>
                  ))}
                </tr>
              </Fragment>
            );
          })}
          {slot !== null && slot === others.length && line}
        </tbody>
      </table>
    </div>
  );
}
