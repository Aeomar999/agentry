import { Fragment } from "react";
import { Link } from "react-router-dom";
import { ChevronRight } from "lucide-react";

export interface Crumb {
  label: string;
  to?: string;
}

export function Breadcrumbs({ crumbs }: { crumbs: Crumb[] }) {
  return (
    <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-1 text-[13px]">
      {crumbs.map((crumb, i) => (
        <Fragment key={i}>
          {i > 0 && <ChevronRight className="size-3.5 shrink-0 text-ink-faint" aria-hidden />}
          {crumb.to && i < crumbs.length - 1 ? (
            <Link to={crumb.to} className="truncate text-ink-muted transition-colors hover:text-ink-primary">
              {crumb.label}
            </Link>
          ) : (
            <span className="truncate font-medium text-ink-primary">{crumb.label}</span>
          )}
        </Fragment>
      ))}
    </nav>
  );
}
