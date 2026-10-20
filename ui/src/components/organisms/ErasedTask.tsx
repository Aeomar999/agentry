import { Trash2 } from "lucide-react";
import type { Erasure } from "@/api/types";
import { LeftBehind } from "@/components/molecules/LeftBehind";
import { Warnings } from "@/components/molecules/Warnings";
import { dateTime } from "@/lib/time";

// ErasedTask is the page of a task erased for good: just erased here, with
// the time and what may be left, or read again, when the server answers
// only that it was erased and on what day (said).
export function ErasedTask({ id, erasure, said }: { id: string; erasure?: Erasure; said?: string }) {
  const title = erasure ? `${id} was erased at ${dateTime(erasure.at)}` : (said ?? `${id} was erased`);
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-6 py-14">
      <div className="flex flex-col items-center gap-2 text-center">
        <Trash2 className="size-6 text-ink-faint" aria-hidden />
        <h1 className="text-[15px] font-semibold text-ink-primary">{title}</h1>
        <p className="text-[12px] text-ink-muted">
          Its history and archive are gone from Agentry, with the shared files no other task names, and nothing brings them back.
        </p>
      </div>
      {erasure && <Warnings warnings={erasure.warnings} />}
      <LeftBehind before={erasure ? dateTime(erasure.at) : "then"} what="it" />
    </div>
  );
}
