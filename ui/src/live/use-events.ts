import { useCallback, useSyncExternalStore } from "react";
import { useEventPages } from "./LiveProvider";
import { EventList, EventsWanted, keyOf, noEvents } from "./event-pages";

export interface Events extends EventList {
  loadEarlier: () => void;
}

const unwatched = () => () => {};

// useEvents shows a task's events, kept up with the log as it moves; every
// view showing the same list shares its reads. null reads nothing yet.
export function useEvents(wanted: EventsWanted | null): Events {
  const pages = useEventPages();
  const key = wanted && keyOf(wanted);
  // Keyed on the list, not on wanted itself: the newest page's bound only
  // says where the first read starts, so a later one changes nothing.
  const watch = useCallback((changed: () => void) => pages.watch(wanted!, changed), [pages, key]);
  const loadEarlier = useCallback(() => wanted && pages.loadEarlier(wanted), [pages, key]);
  const list = useSyncExternalStore(wanted ? watch : unwatched, () => (wanted ? pages.snapshot(wanted) : noEvents));
  return { ...list, loadEarlier };
}
