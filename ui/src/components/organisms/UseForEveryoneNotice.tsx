import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { turnOffUseForEveryone } from "@/api/client";
import { usePolicyChange } from "./usePolicyChange";

// use_for_everyone puts one profile in place of every agent's own. Wherever
// agents' profiles show, this says so, with the way to turn it off: the
// line is commented out, so each agent uses its own profile again.
export function UseForEveryoneNotice({ profile, hash, onChanged }: { profile: string; hash: string; onChanged: () => void }) {
  const { busy, run } = usePolicyChange(onChanged);
  return (
    <Banner tone="info" title={`Every agent uses ${profile}; their own profiles are ignored`}>
      <div className="flex flex-wrap items-center gap-3">
        <span className="min-w-0 flex-1">
          policy.toml sets <Mono>use_for_everyone = "{profile}"</Mono>. Turn it off and each agent uses its own profile again.
        </span>
        <Button size="sm" disabled={busy} onClick={() => run("Turned off use_for_everyone", () => turnOffUseForEveryone(hash))}>
          Turn it off
        </Button>
      </div>
    </Banner>
  );
}
