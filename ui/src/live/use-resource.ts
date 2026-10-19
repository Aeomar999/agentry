import { useCallback, useSyncExternalStore } from "react";
import { useResourceCache } from "./LiveProvider";

export interface Resource<T> {
  data: T | undefined;
  error: string;
  reload: () => void;
}

const unwatched = () => () => {};
const nothing = { data: undefined, error: "" };

// useResource reads one API path and reads it again whenever the log moves
// past what it was read at, so what's on screen is never older than the
// last event. Every component showing the path shares one read of it.
export function useResource<T>(path: string | null): Resource<T> {
  const cache = useResourceCache();
  const watch = useCallback((changed: () => void) => cache.watch(path!, changed), [cache, path]);
  const snapshot = useSyncExternalStore(path ? watch : unwatched, () => (path ? cache.snapshot(path) : nothing));
  const reload = useCallback(() => path && cache.reload(path), [cache, path]);
  return { data: snapshot.data as T | undefined, error: snapshot.error, reload };
}
