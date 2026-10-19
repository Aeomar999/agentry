import type { LogEvent } from "@/api/types";
import type { Fetch } from "./resource-cache";

// A task's events, read a page at a time from /api/tasks/{task}/events, for
// the views that show them: the chat with the coordinator, the Activity tab
// and the Events tab. The reads keep the resource cache's rules: one read of
// a list at a time however many views show it, read again only when the log
// has moved past what it holds, and never aborted. A list moves by pages, so
// it can't be one path of the cache: each read asks for the page after (or
// before) the events it holds.

// What a view shows of a task's events: those of some types (every type
// when none are given), either all of them from the start, or the newest
// page, the events before newestBefore, with earlier pages when asked for.
export interface EventsWanted {
  task: string;
  types?: readonly string[];
  newestBefore?: number;
}

export interface EventList {
  events: LogEvent[]; // oldest first
  loading: boolean; // until the first page comes, or every page from the start
  earlier: boolean; // more events lie before the oldest held: loadEarlier reads them
  loadingEarlier: boolean;
  error: string;
}

export const noEvents: EventList = { events: [], loading: true, earlier: false, loadingEarlier: false, error: "" };

interface Page {
  events: LogEvent[];
  more: boolean; // more events lie beyond the page, on the side it was read from
}

interface Due {
  side: "first" | "after" | "before";
  path: string;
}

interface List {
  wanted: EventsWanted;
  shown: EventList;
  position: number; // the log position it holds every event up to; -1 before the first read
  started: boolean; // its first page has come
  ahead: boolean; // the last page read forwards said more lie after it
  earlierAsked: boolean;
  reading: boolean;
  watchers: Set<() => void>;
}

// keyOf names a list: the newest page's bound isn't part of it, as it only
// says where the first read starts.
export function keyOf(wanted: EventsWanted): string {
  return `${wanted.task}${typesQuery(wanted.types)}${wanted.newestBefore === undefined ? " from the start" : " newest first"}`;
}

export class EventPages {
  private lists = new Map<string, List>();
  private live = 0;

  constructor(private fetch: Fetch) {}

  // watch shows a list: it is read now unless it is already held, and read
  // on as the log moves, until the last watcher stops.
  watch(wanted: EventsWanted, changed: () => void): () => void {
    const key = keyOf(wanted);
    let list = this.lists.get(key);
    if (!list) {
      list = {
        wanted,
        shown: noEvents,
        position: -1,
        started: false,
        ahead: false,
        earlierAsked: false,
        reading: false,
        watchers: new Set(),
      };
      this.lists.set(key, list);
    }
    list.watchers.add(changed);
    this.readIfDue(key, list);
    return () => {
      list.watchers.delete(changed);
      this.forgetIfUnwatched(key, list);
    };
  }

  snapshot(wanted: EventsWanted): EventList {
    return this.lists.get(keyOf(wanted))?.shown ?? noEvents;
  }

  // moved is the log's newest position, from the long poll.
  moved(position: number) {
    this.live = position;
    this.lists.forEach((list, key) => this.readIfDue(key, list));
  }

  // loadEarlier reads the page before the oldest event held, now or once
  // the read in flight is back.
  loadEarlier(wanted: EventsWanted) {
    const key = keyOf(wanted);
    const list = this.lists.get(key);
    if (!list || !list.shown.earlier || list.earlierAsked) return;
    list.earlierAsked = true;
    this.show(list, { loadingEarlier: true });
    this.readIfDue(key, list);
  }

  // due is the page a list needs read now, and from which side, or null.
  private due(list: List): Due | null {
    const { task, types, newestBefore } = list.wanted;
    const path = `/api/tasks/${encodeURIComponent(task)}/events${typesQuery(types)}`;
    const page = (bound: string) => `${path}${path.includes("?") ? "&" : "?"}${bound}`;
    const held = list.shown.events;
    if (!list.started) {
      if (list.position >= this.live) return null; // its first read failed: tried again when the log moves
      return { side: "first", path: page(newestBefore === undefined ? "after=0" : `before=${newestBefore}`) };
    }
    // Forwards from the newest event held, not from the position: a read
    // that failed half way leaves nothing out.
    const newest = held.length ? held[held.length - 1].position : list.position;
    if (list.ahead) return { side: "after", path: page(`after=${newest}`) };
    if (list.earlierAsked) return { side: "before", path: page(`before=${held[0].position}`) };
    if (list.position < this.live) return { side: "after", path: page(`after=${newest}`) };
    return null;
  }

  private readIfDue(key: string, list: List) {
    if (list.reading || list.watchers.size === 0) return;
    const due = this.due(list);
    if (due) this.read(key, list, due);
  }

  private async read(key: string, list: List, { side, path }: Due) {
    list.reading = true;
    const asked = this.live;
    try {
      const read = await this.fetch(path);
      const page = read.data as Page;
      // Never below where it was asked, as in the resource cache.
      const readAt = Math.max(read.position ?? asked, asked);
      if (side === "first") this.first(list, page, readAt);
      else if (side === "after") this.append(list, page, readAt);
      else this.prepend(list, page);
    } catch (failure) {
      if (side === "before") list.earlierAsked = false;
      else {
        list.position = Math.max(list.position, asked); // tried again when the log moves
        list.ahead = false;
      }
      this.show(list, { loadingEarlier: list.earlierAsked, error: (failure as Error).message });
    }
    list.reading = false;
    this.readIfDue(key, list);
    this.forgetIfUnwatched(key, list);
  }

  private first(list: List, page: Page, readAt: number) {
    list.started = true;
    const { newestBefore } = list.wanted;
    if (newestBefore === undefined) {
      this.append(list, page, readAt);
      return;
    }
    list.position = newestBefore - 1;
    this.show(list, { events: page.events, loading: false, earlier: page.more, error: "" });
  }

  // append adds a page read forwards. Once none lie ahead of it, every event
  // up to where the server read is held.
  private append(list: List, page: Page, readAt: number) {
    const held = list.shown.events;
    const newest = held.length ? held[held.length - 1].position : 0;
    list.ahead = page.more;
    if (!page.more) list.position = Math.max(list.position, readAt);
    const events = [...held, ...page.events.filter((e) => e.position > newest)];
    this.show(list, { events, loading: list.shown.loading && page.more, error: "" });
  }

  private prepend(list: List, page: Page) {
    list.earlierAsked = false;
    const oldest = list.shown.events[0].position;
    const events = [...page.events.filter((e) => e.position < oldest), ...list.shown.events];
    this.show(list, { events, earlier: page.more, loadingEarlier: false, error: "" });
  }

  private show(list: List, change: Partial<EventList>) {
    list.shown = { ...list.shown, ...change };
    list.watchers.forEach((changed) => changed());
  }

  // A list nobody shows is dropped once its read is over, as in the
  // resource cache; showing it again reads it again.
  private forgetIfUnwatched(key: string, list: List) {
    if (list.watchers.size === 0 && !list.reading) this.lists.delete(key);
  }
}

function typesQuery(types: readonly string[] = []): string {
  return types.length ? `?${types.map((type) => `type=${encodeURIComponent(type)}`).join("&")}` : "";
}
