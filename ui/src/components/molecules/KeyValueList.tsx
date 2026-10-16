import { ReactNode } from "react";

export interface KeyValue {
  label: string;
  value: ReactNode;
}

export function KeyValueList({ items }: { items: KeyValue[] }) {
  return (
    <dl className="grid grid-cols-[96px_minmax(0,1fr)] gap-x-3 gap-y-2 text-[12px]">
      {items.map((item) => (
        <div key={item.label} className="contents">
          <dt className="text-ink-muted">{item.label}</dt>
          <dd className="min-w-0 break-words text-ink-primary">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}
