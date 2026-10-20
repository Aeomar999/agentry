import { ReactNode, Ref } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/atoms/Button";

export interface HeaderAction {
  label: string;
  icon?: ReactNode;
  to?: string; // navigates
  onClick?: () => void;
  primary?: boolean; // the one orange button on the screen
  disabled?: boolean;
  ref?: Ref<HTMLButtonElement>; // to focus the button again later
}

interface PageHeaderProps {
  title: ReactNode;
  meta?: ReactNode; // one muted line under the title
  badges?: ReactNode;
  actions?: HeaderAction[];
}

export function PageHeader({ title, meta, badges, actions = [] }: PageHeaderProps) {
  const navigate = useNavigate();
  return (
    <header className="flex flex-wrap items-start justify-between gap-3 border-b border-divider px-6 pt-5 pb-4">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          {/* The title can take focus (not by Tab), as when what had it has gone. */}
          <h1
            tabIndex={-1}
            className="font-display text-[22px] leading-7 font-bold tracking-tight break-words text-ink-primary [font-stretch:85%] outline-none"
          >
            {title}
          </h1>
          {badges}
        </div>
        {meta && <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-ink-muted">{meta}</div>}
      </div>
      {actions.length > 0 && (
        <div className="flex shrink-0 items-center gap-2">
          {actions.map((action) => (
            <Button
              key={action.label}
              ref={action.ref}
              variant={action.primary ? "primary" : "secondary"}
              icon={action.icon}
              disabled={action.disabled}
              onClick={() => (action.to ? navigate(action.to) : action.onClick?.())}
            >
              {action.label}
            </Button>
          ))}
        </div>
      )}
    </header>
  );
}
