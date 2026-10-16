import { useCallback, useEffect, useState } from "react";
import { api } from "@/api/client";
import { useLive } from "./LiveProvider";

export interface Resource<T> {
  data: T | undefined;
  error: string;
  reload: () => void;
}

// useResource reads one API path and reads it again whenever the log moves,
// so what's on screen is never older than the last event.
export function useResource<T>(path: string | null): Resource<T> {
  const { position } = useLive();
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const [reloads, setReloads] = useState(0);

  useEffect(() => {
    setData(undefined);
    setError("");
  }, [path]);

  useEffect(() => {
    if (!path) return;
    const stop = new AbortController();
    api<T>("GET", path, undefined, stop.signal)
      .then((value) => {
        setData(value);
        setError("");
      })
      .catch((failure: Error) => {
        if (!stop.signal.aborted) setError(failure.message);
      });
    return () => stop.abort();
  }, [path, position, reloads]);

  const reload = useCallback(() => setReloads((n) => n + 1), []);
  return { data, error, reload };
}
