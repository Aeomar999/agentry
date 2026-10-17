import { useState } from "react";
import { CommandLine } from "./CommandLine";
import { SegmentedControl } from "./SegmentedControl";

// An install command to run on another machine, for its system: macOS and
// Linux run it in a shell, Windows in PowerShell (irm … | iex, as Claude
// Code's own install). It starts on the system
// this browser runs on, which is often the machine.

type System = "unix" | "windows";

const thisSystem = (): System => (/Windows/i.test(navigator.userAgent) ? "windows" : "unix");

export function InstallCommand({ command, windowsCommand }: { command: string; windowsCommand: string }) {
  const [system, setSystem] = useState<System>(thisSystem);
  return (
    <div className="flex flex-col gap-2">
      <SegmentedControl
        segments={[
          { id: "unix", label: "macOS / Linux" },
          { id: "windows", label: "Windows" },
        ]}
        active={system}
        onChange={(id) => setSystem(id as System)}
      />
      <CommandLine command={system === "windows" ? windowsCommand : command} />
      <p className="text-[11px] text-ink-faint">{system === "windows" ? "In PowerShell." : "In a terminal."}</p>
    </div>
  );
}
