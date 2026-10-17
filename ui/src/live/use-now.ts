import { useEffect, useState } from "react";

// useNow is the time, again every second while running is true: for the
// clocks of things still happening.
export function useNow(running: boolean): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!running) return;
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, [running]);
  return now;
}
