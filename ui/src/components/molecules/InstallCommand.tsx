import { useState } from "react";
import { Select } from "@/components/atoms/Select";
import type { InstallCommands } from "@/api/types";
import { CommandLine } from "./CommandLine";
import { SegmentedControl } from "./SegmentedControl";

// An install command to run on another machine, for its system: macOS and
// Linux run it in a shell, Windows in PowerShell (irm … | iex, as Claude
// Code's own install). It starts on the system
// this browser runs on, which is often the machine. The server has an
// address per network, so the command comes for each: pick the one the
// machine reaches (hidden when there is only one).

type System = "unix" | "windows";

const thisSystem = (): System => (/Windows/i.test(navigator.userAgent) ? "windows" : "unix");

export function InstallCommand({ commands }: { commands: InstallCommands }) {
  const [system, setSystem] = useState<System>(thisSystem);
  const [url, setURL] = useState(commands.address);
  const shown = commands.addresses.find((a) => a.url === url) ?? commands;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <SegmentedControl
          segments={[
            { id: "unix", label: "macOS / Linux" },
            { id: "windows", label: "Windows" },
          ]}
          active={system}
          onChange={(id) => setSystem(id as System)}
        />
        {commands.addresses.length > 1 && (
          <Select
            aria-label="The address the machine reaches"
            className="h-7 w-auto min-w-0 flex-1 text-[12px]"
            value={url}
            onChange={(e) => setURL(e.target.value)}
          >
            {commands.addresses.map((a) => (
              <option key={a.url} value={a.url}>
                {a.url.replace(/^https?:\/\//, "")} · {a.label}
              </option>
            ))}
          </Select>
        )}
      </div>
      <CommandLine command={system === "windows" ? shown.windowsCommand : shown.command} />
      <p className="text-[11px] text-ink-faint">{system === "windows" ? "In PowerShell." : "In a terminal."}</p>
    </div>
  );
}
