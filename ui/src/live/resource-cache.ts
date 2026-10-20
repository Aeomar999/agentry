// The page's reads of the API, shared by every component that shows the
// same path. Each reply says the log position it was read at, so a path is
// read again only when the log has moved past it, and once per move however
// many events arrive while it is being read. A read is never aborted: the
// server would finish it anyway, and a browser that drops the connection
// opens a new one for the next read. A server that restarts on a shorter
// log has the whole cache read again (LogPositions).

import { ApiError } from "@/api/client";
import { LogPositions } from "./log-positions";

export interface Read {
  data: unknown;
  position?: number; // of the log, when the server read it; missing when the reply didn't say
}

export type Fetch = (path: string) => Promise<Read>;

export interface Snapshot {
  data: unknown;
  error: string;
  status: number; // the server's answer to the read that failed, such as 410 for a task erased; 0 when none did
  position: number; // of the log, that the data is as fresh as; -1 before the first read
}

export const nothing: Snapshot = { data: undefined, error: "", status: 0, position: -1 };

interface Entry {
  snapshot: Snapshot;
  position: number; // the log position the snapshot is as fresh as; -1 before the first read
  reading: boolean;
  reloadAsked: boolean; // reload() while reading: read again when it returns
  reloaded: (() => void)[]; // reloads waiting for the next read to start, and return
  watchers: Set<() => void>;
}

export class ResourceCache {
  private entries = new Map<string, Entry>();
  private live = 0;
  private log = new LogPositions((position) => this.reset(position));

  constructor(private fetch: Fetch) {}

  // watch shows a path: it is read now unless it is already fresh, and
  // again as the log moves, until the last watcher stops.
  watch(path: string, changed: () => void): () => void {
    let entry = this.entries.get(path);
    if (!entry) {
      entry = { snapshot: nothing, position: -1, reading: false, reloadAsked: false, reloaded: [], watchers: new Set() };
      this.entries.set(path, entry);
    }
    entry.watchers.add(changed);
    this.readIfOld(path, entry);
    return () => {
      entry.watchers.delete(changed);
      this.forgetIfUnwatched(path, entry);
    };
  }

  snapshot(path: string): Snapshot {
    return this.entries.get(path)?.snapshot ?? nothing;
  }

  // moved is the log's newest position, from the long poll.
  moved(position: number) {
    if (!this.log.moved(position)) return;
    this.live = position;
    this.entries.forEach((entry, path) => this.readIfOld(path, entry));
  }

  // reload reads a path again whatever its position, as after an action
  // whose result the page wants to see at once. It returns once a read
  // that started after it is shown, so the page can tell what the action
  // left from what was read before it.
  reload(path: string): Promise<void> {
    const entry = this.entries.get(path);
    if (!entry) return Promise.resolve();
    const shown = new Promise<void>((resolve) => entry.reloaded.push(resolve));
    if (entry.reading) entry.reloadAsked = true;
    else this.read(path, entry);
    return shown;
  }

  // reset reads every path shown again, as of the shorter log's position.
  // Each keeps showing its old data until its new read is back, so the page
  // doesn't flash empty. Paths nobody shows are already forgotten, or will
  // be when their read in flight returns.
  private reset(position: number) {
    this.live = position;
    this.entries.forEach((entry, path) => {
      entry.position = -1;
      this.readIfOld(path, entry);
    });
  }

  private readIfOld(path: string, entry: Entry) {
    if (!entry.reading && entry.watchers.size > 0 && entry.position < this.live) this.read(path, entry);
  }

  private async read(path: string, entry: Entry) {
    entry.reading = true;
    entry.reloadAsked = false;
    const reloads = entry.reloaded.splice(0);
    const asked = this.live;
    const sent = this.log.sent();
    try {
      const read = await this.fetch(path);
      if (this.log.replied(sent, read.position)) {
        // A reply with no position is as fresh as the position it was asked
        // at, so it isn't read again until the log moves, instead of in a loop.
        entry.position = read.position ?? asked;
        this.show(entry, { data: read.data, error: "", status: 0, position: entry.position });
      }
    } catch (failure) {
      if (!this.log.stale(sent)) {
        // Tried as of the position it was asked at: the next move tries again.
        entry.position = Math.max(entry.position, asked);
        this.show(entry, { ...entry.snapshot, error: (failure as Error).message, status: statusOf(failure) });
      }
    }
    entry.reading = false;
    // A read sent before a reset is thrown away, and read again below: the
    // reloads waiting for it wait for that read instead.
    if (this.log.stale(sent)) entry.reloaded.unshift(...reloads);
    else reloads.forEach((shown) => shown());
    if (entry.reloadAsked && entry.watchers.size > 0) this.read(path, entry);
    else this.readIfOld(path, entry);
    this.forgetIfUnwatched(path, entry);
  }

  private show(entry: Entry, snapshot: Snapshot) {
    entry.snapshot = snapshot;
    entry.watchers.forEach((changed) => changed());
  }

  // A path nobody shows is dropped once its read is over, so the cache
  // holds only what is on screen; showing it again reads it again.
  private forgetIfUnwatched(path: string, entry: Entry) {
    if (entry.watchers.size === 0 && !entry.reading) {
      entry.reloaded.splice(0).forEach((shown) => shown()); // nobody will read it again
      this.entries.delete(path);
    }
  }
}

// statusOf is the server's answer to a read it refused, or 0 for a read
// that never got one, such as a dropped connection.
function statusOf(failure: unknown): number {
  return failure instanceof ApiError ? failure.status : 0;
}
