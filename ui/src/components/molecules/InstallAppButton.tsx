import { useEffect, useRef, useState } from "react";
import { Download } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { install, useInstall } from "@/lib/install";

// Install app, in the top bar: installs the page as an app where the browser
// offers it. Opened over plain http at a network address, where browsers
// don't install pages, it says where you can instead. Hidden in the app
// itself, and where the browser offers nothing.
export function InstallAppButton() {
  const state = useInstall();
  const [noteOpen, setNoteOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!noteOpen) return;
    const close = (event: Event) => {
      if (event instanceof KeyboardEvent && event.key !== "Escape") return;
      if (event.type === "mousedown" && box.current?.contains(event.target as Node)) return;
      setNoteOpen(false);
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", close);
    };
  }, [noteOpen]);

  if (state === "installed" || state === "unavailable") return null;
  const port = location.port ? ":" + location.port : "";
  return (
    <div ref={box} className="relative">
      <Button
        size="sm"
        icon={<Download />}
        aria-label="Install app"
        aria-expanded={state === "insecure" ? noteOpen : undefined}
        onClick={() => (state === "ready" ? install() : setNoteOpen((open) => !open))}
      >
        <span className="max-sm:hidden">Install app</span>
      </Button>
      {noteOpen && (
        <p
          role="note"
          className="absolute top-full right-0 mt-2 w-72 rounded-md border border-divider-2 bg-surface-2 p-3 text-[12.5px] text-ink-secondary"
        >
          Browsers install apps only from secure pages. Open Agentry at{" "}
          <span className="font-mono text-ink-primary">http://localhost{port}</span> on the computer it runs on, or over https, and install
          it there.
        </p>
      )}
    </div>
  );
}
