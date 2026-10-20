import { useCallback, useSyncExternalStore } from "react";
import { useResourceCache } from "./LiveProvider";
import { nothing } from "./resource-cache";

export interface Resource<T> {
  data: T | undefined;
  error: string;
  status: number; // the server's answer to the read that failed, such as 410 for a task erased; 0 when none did
  position: number; // of the log, that data is as fresh as; -1 before the first read
  reload: () => Promise<void>; // returns once a read started after it is shown
}

const unwatched = () => () => {};

// useResource reads one API path and reads it again whenever the log moves
// past what it was read at, so what's on screen is never older than the
// last event. Every component showing the path shares one read of it.
export function useResource<T>(path: string | null): Resource<T> {
  const cache = useResourceCache();
  const watch = useCallback((changed: () => void) => cache.watch(path!, changed), [cache, path]);
  const snapshot = useSyncExternalStore(path ? watch : unwatched, () => (path ? cache.snapshot(path) : nothing));
  const reload = useCallback(() => (path ? cache.reload(path) : Promise.resolve()), [cache, path]);
  return { data: snapshot.data as T | undefined, error: snapshot.error, status: snapshot.status, position: snapshot.position, reload };
}
