import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn joins class names; when two Tailwind classes conflict, the later wins.
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
