import { ReactNode } from "react";
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
}

// Dense rows (36px), hairline separated, the active row a flat lighter
// surface. Rows open on click or Enter. Cells don't wrap; the
// one wide column (w-full max-w-0) truncates or clamps its own text.
export function ResourceTable<Row>({ columns, rows, rowKey, onOpen, activeKey, empty }: ResourceTableProps<Row>) {
  if (rows.length === 0 && empty) return <>{empty}</>;
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
        <tbody>
          {rows.map((row) => {
            const key = rowKey(row);
            const active = key === activeKey;
            return (
              <tr
                key={key}
                tabIndex={onOpen ? 0 : undefined}
                onClick={() => onOpen?.(row)}
                onKeyDown={(event) => event.key === "Enter" && onOpen?.(row)}
                data-active={active || undefined}
                className={cn(
                  "h-row-list border-b border-divider transition-colors duration-(--dur-fast)",
                  onOpen && "cursor-pointer hover:bg-surface-2",
                  active && "bg-surface-3",
                )}
              >
                {columns.map((column) => (
                  <td key={column.header} className={cn("px-3 py-1.5 align-middle whitespace-nowrap first:pl-4", column.className)}>
                    {column.render(row)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
