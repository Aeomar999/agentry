import { FormEvent, useState } from "react";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { DecisionBadge } from "@/components/molecules/StatusBadges";
import { Mono } from "@/components/molecules/Mono";
import { tryPolicyCall } from "@/api/client";
import type { TriedCall, Verdict } from "@/api/types";

// The tools most calls use; any other name can be typed.
const TOOLS = ["Bash", "Read", "Edit", "Write", "Glob", "Grep", "WebFetch"];

// TryPolicyCall asks what a profile would decide about one call, before
// anyone relies on it. onTried gets the reply, or undefined once it no
// longer matches what is typed, so the panel can mark the deciding rule.
export function TryPolicyCall({ profile, onTried }: { profile: string; onTried: (tried?: TriedCall) => void }) {
  const [tool, setTool] = useState("Bash");
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState<TriedCall>();
  const [failure, setFailure] = useState("");
  const tools = `try-tools-${profile}`;

  function show(next?: TriedCall) {
    setTried(next);
    onTried(next);
  }
  function edit(change: () => void) {
    change();
    show(undefined);
    setFailure("");
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    show(undefined);
    setFailure("");
    setBusy(true);
    try {
      show(await tryPolicyCall({ profile, tool: tool.trim(), input }));
    } catch (failed) {
      setFailure(failed instanceof Error ? failed.message : String(failed));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-1.5 border-b border-divider px-4 py-2.5 text-[12px]">
      <form className="flex items-center gap-2" onSubmit={submit} aria-label={`Try a call against ${profile}`}>
        <span className="shrink-0 text-ink-muted">Try a call</span>
        <Input
          mono
          className="w-28 shrink-0"
          aria-label="Tool"
          list={tools}
          value={tool}
          onChange={(event) => edit(() => setTool(event.target.value))}
        />
        <datalist id={tools}>
          {TOOLS.map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>
        <Input
          mono
          aria-label="Command or path"
          placeholder="git push origin main, src/main.go"
          value={input}
          onChange={(event) => edit(() => setInput(event.target.value))}
        />
        <Button type="submit" disabled={busy || tool.trim() === ""}>
          Try
        </Button>
      </form>
      {failure && <p className="text-danger">{failure}</p>}
      {tried && <TriedLines tried={tried} />}
    </div>
  );
}

// TriedLines say what the profile decides, where the call was taken to
// run, and, while use_for_everyone puts another profile in its place, what
// really decides today.
function TriedLines({ tried }: { tried: TriedCall }) {
  return (
    <>
      <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <VerdictLine verdict={tried.verdict} />
        <Mono className="text-ink-faint">as if run in {tried.workspace}</Mono>
      </p>
      {tried.override && (
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-info">
          <span>
            But today <Mono>{tried.override.profile}</Mono> decides for every agent:
          </span>
          <VerdictLine verdict={tried.override.verdict} />
        </p>
      )}
    </>
  );
}

function VerdictLine({ verdict }: { verdict: Verdict }) {
  return (
    <>
      <DecisionBadge decision={verdict.decision} />
      <span className="font-medium text-ink-primary">{verdict.rule}</span>
      <span className="text-ink-muted">{verdict.why}</span>
    </>
  );
}
