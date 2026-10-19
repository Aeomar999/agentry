import { ButtonHTMLAttributes, forwardRef, ReactNode } from "react";
import { cn } from "@/lib/cn";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: Size;
  icon?: ReactNode;
}

const VARIANTS: Record<ButtonVariant, string> = {
  // The focus line stands 2px off the orange fill, or the two would merge.
  primary: "bg-brand text-white hover:bg-brand-hover focus-visible:outline-offset-2",
  secondary: "bg-surface-2 text-ink-primary border border-divider hover:border-divider-2 hover:bg-surface-3",
  ghost: "bg-transparent text-ink-secondary hover:bg-surface-2 hover:text-ink-primary",
  danger: "bg-transparent text-danger border border-divider hover:border-danger hover:bg-danger-soft",
};

const SIZES: Record<Size, string> = {
  sm: "h-7 px-2.5 text-[12px] gap-1.5",
  md: "h-8 px-3 text-[13px] gap-2",
};

// Secondary is the default: orange is for the one main action on a screen.
// type="button" by default, so a button never submits a form by accident.
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", icon, className, type = "button", children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-md font-medium whitespace-nowrap",
        "cursor-pointer transition-colors duration-(--dur-fast) ease-(--ease-snap)",
        "disabled:cursor-not-allowed disabled:opacity-40",
        VARIANTS[variant],
        SIZES[size],
        className,
      )}
      {...rest}
    >
      {icon && <span className="shrink-0 [&>svg]:size-3.5">{icon}</span>}
      {children}
    </button>
  );
});
