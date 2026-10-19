import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { api } from "@/api/client";
import { readPath } from "./read-path";
import { ResourceCache } from "./resource-cache";
import { EventPages } from "./event-pages";

// The page stays live by long-polling the log: the server answers when the
// log moves past our position (or after 25 s), and the resources on screen
// that are older than the new position are read again. One poll for the
// whole page.

export type Connection = "connecting" | "live" | "reconnecting";

interface Live {
  position: number;
  connection: Connection;
}

const LiveContext = createContext<Live>({ position: 0, connection: "connecting" });
// Its own context, so a resource doesn't render again for every move of the
// log, only when its own data changes.
const CacheContext = createContext<ResourceCache>(new ResourceCache(readPath));
const EventPagesContext = createContext<EventPages>(new EventPages(readPath));

export function LiveProvider({ children }: { children: ReactNode }) {
  const [live, setLive] = useState<Live>({ position: 0, connection: "connecting" });
  const [cache] = useState(() => new ResourceCache(readPath));
  const [pages] = useState(() => new EventPages(readPath));

  useEffect(() => {
    const stop = new AbortController();
    async function poll() {
      let position = 0;
      while (!stop.signal.aborted) {
        try {
          const change = await api<{ position: number }>("GET", `/api/changes?after=${position}`, undefined, stop.signal);
          position = change.position;
          cache.moved(position);
          pages.moved(position);
          setLive({ position, connection: "live" });
        } catch (error) {
          if (stop.signal.aborted) return;
          setLive((current) => ({ ...current, connection: "reconnecting" })); // a 401 shows the sign-in page instead (SignedIn)
          await new Promise((resume) => setTimeout(resume, 2000));
        }
      }
    }
    poll();
    return () => stop.abort();
  }, [cache, pages]);

  return (
    <CacheContext.Provider value={cache}>
      <EventPagesContext.Provider value={pages}>
        <LiveContext.Provider value={live}>{children}</LiveContext.Provider>
      </EventPagesContext.Provider>
    </CacheContext.Provider>
  );
}

export function useLive(): Live {
  return useContext(LiveContext);
}

export function useResourceCache(): ResourceCache {
  return useContext(CacheContext);
}

export function useEventPages(): EventPages {
  return useContext(EventPagesContext);
}
