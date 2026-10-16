import { ReactNode } from "react";
import { cn } from "@/lib/cn";

// Ids, paths, branches and times: anything compared character by character.
export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn("font-mono text-[12px]", className)}>{children}</span>;
}
