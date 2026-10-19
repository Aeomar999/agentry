// The page's reads of the API, shared by every component that shows the
// same path. Each reply says the log position it was read at, so a path is
// read again only when the log has moved past it, and once per move however
// many events arrive while it is being read. A read is never aborted: the
// server would finish it anyway, and a browser that drops the connection
// opens a new one for the next read.

export interface Read {
  data: unknown;
  position: number; // of the log, when the server read it
}

export type Fetch = (path: string) => Promise<Read>;

export interface Snapshot {
  data: unknown;
  error: string;
}

const nothing: Snapshot = { data: undefined, error: "" };

interface Entry {
  snapshot: Snapshot;
  position: number; // the log position the snapshot is as fresh as; -1 before the first read
  reading: boolean;
  reloadAsked: boolean; // reload() while reading: read again when it returns
  watchers: Set<() => void>;
}

export class ResourceCache {
  private entries = new Map<string, Entry>();
  private live = 0;

  constructor(private fetch: Fetch) {}

  // watch shows a path: it is read now unless it is already fresh, and
  // again as the log moves, until the last watcher stops.
  watch(path: string, changed: () => void): () => void {
    let entry = this.entries.get(path);
    if (!entry) {
      entry = { snapshot: nothing, position: -1, reading: false, reloadAsked: false, watchers: new Set() };
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
    this.live = position;
    this.entries.forEach((entry, path) => this.readIfOld(path, entry));
  }

  // reload reads a path again whatever its position, as after an action
  // whose result the page wants to see at once.
  reload(path: string) {
    const entry = this.entries.get(path);
    if (!entry) return;
    if (entry.reading) entry.reloadAsked = true;
    else this.read(path, entry);
  }

  private readIfOld(path: string, entry: Entry) {
    if (!entry.reading && entry.watchers.size > 0 && entry.position < this.live) this.read(path, entry);
  }

  private async read(path: string, entry: Entry) {
    entry.reading = true;
    entry.reloadAsked = false;
    const asked = this.live;
    try {
      const read = await this.fetch(path);
      entry.position = read.position;
      this.show(entry, { data: read.data, error: "" });
    } catch (failure) {
      // Tried as of the position it was asked at: the next move tries again.
      entry.position = Math.max(entry.position, asked);
      this.show(entry, { data: entry.snapshot.data, error: (failure as Error).message });
    }
    entry.reading = false;
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
    if (entry.watchers.size === 0 && !entry.reading) this.entries.delete(path);
  }
}
