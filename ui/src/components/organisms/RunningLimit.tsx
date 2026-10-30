import { useState } from "react";
import { Check } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { LimitSelect } from "@/components/molecules/LimitSelect";
import { Mono } from "@/components/molecules/Mono";
import { changeSettings } from "@/api/client";
import type { Settings } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { CUSTOM, LimitChoice, RUNNING, choiceOf, limitOf, runningLine } from "@/lib/limits";
import { useToast } from "@/shell/Toasts";

// The most tasks Agentry gives a turn at once, across every repo, and how
// many run against it and wait for a slot. A preset or No limit saves as you
// pick it; a number of your own once you save it. As the ideas settings do,
// it shows what you chose while the server saves it, and what it saved until
// the settings load again, which they do whenever the event log moves, so
// the running count keeps up as turns start and end.
export function RunningLimit({ queued }: { queued: number }) {
  const notify = useToast();
  const { data, error } = useResource<Settings>("/api/settings");
  const [saving, setSaving] = useState<number | null>(null);
  const [saved, setSaved] = useState<Settings | null>(null);
  const [typing, setTyping] = useState<LimitChoice | null>(null); // Custom, until it is saved
  const [seen, setSeen] = useState(data);
  if (seen !== data) {
    setSeen(data);
    setSaved(null);
  }

  const limit = saving ?? saved?.maxRunningTasks ?? data?.maxRunningTasks ?? 0;
  const running = saved?.runningTasks ?? data?.runningTasks ?? 0;
  const choice = typing ?? choiceOf(RUNNING, limit);
  const typed = typing ? limitOf(RUNNING, typing) : NaN;
  const busy = saving !== null || !data;

  async function save(next: number) {
    setSaving(next);
    try {
      setSaved(await changeSettings({ maxRunningTasks: next }));
      setTyping(null);
    } catch (failure) {
      notify("danger", "The limit wasn't saved", (failure as Error).message);
    } finally {
      setSaving(null);
    }
  }

  const pick = (next: LimitChoice) => {
    if (next.option === CUSTOM) {
      setTyping(next);
      return;
    }
    setTyping(null);
    save(Number(next.option));
  };

  return (
    <span className="flex flex-wrap items-center gap-2 text-[12px] text-ink-muted">
      <label htmlFor="most-running">Most running at once</label>
      <LimitSelect id="most-running" scale={RUNNING} choice={choice} onChange={pick} noLimit="No limit" disabled={busy} />
      {typing && <IconButton icon={<Check />} label="Save the limit" disabled={busy || Number.isNaN(typed)} onClick={() => save(typed)} />}
      {error ? <span className="text-danger">Couldn't read the limit: {error}</span> : <Mono>{runningLine(limit, running, queued)}</Mono>}
    </span>
  );
}
