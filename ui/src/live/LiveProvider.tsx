import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { api, SignedOutError } from "@/api/client";

// The page stays live by long-polling the log: the server answers when the
// log moves past our position (or after 25 s), and every resource on screen
// refetches when the position changes. One poll for the whole page.

export type Connection = "connecting" | "live" | "reconnecting" | "signed out";

interface Live {
  position: number;
  connection: Connection;
}

const LiveContext = createContext<Live>({ position: 0, connection: "connecting" });

export function LiveProvider({ children }: { children: ReactNode }) {
  const [live, setLive] = useState<Live>({ position: 0, connection: "connecting" });

  useEffect(() => {
    const stop = new AbortController();
    async function poll() {
      let position = 0;
      while (!stop.signal.aborted) {
        try {
          const change = await api<{ position: number }>("GET", `/api/changes?after=${position}`, undefined, stop.signal);
          position = change.position;
          setLive({ position, connection: "live" });
        } catch (error) {
          if (stop.signal.aborted) return;
          setLive((current) => ({ ...current, connection: error instanceof SignedOutError ? "signed out" : "reconnecting" }));
          await new Promise((resume) => setTimeout(resume, 2000));
        }
      }
    }
    poll();
    return () => stop.abort();
  }, []);

  return <LiveContext.Provider value={live}>{children}</LiveContext.Provider>;
}

export function useLive(): Live {
  return useContext(LiveContext);
}
