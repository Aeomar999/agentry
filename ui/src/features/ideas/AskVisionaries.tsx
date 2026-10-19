import { Lightbulb } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { type AskVisionaries, useAskVisionaries } from "./visionaries";

// AskVisionariesOffer is the one way to turn Ideas on, asking the
// visionaries, which adds the team first, or that there is no way when no
// team template turns Ideas on.
export function AskVisionariesOffer({ visionaries }: { visionaries: AskVisionaries }) {
  if (!visionaries.ready)
    return <p className="text-ink-faint">This Agentry has no team template that turns Ideas on, so there is no team to ask.</p>;
  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Button variant="primary" icon={<Lightbulb />} disabled={visionaries.adding} onClick={visionaries.ask}>
          {visionaries.adding ? "Adding the team…" : "Ask the visionaries"}
        </Button>
      </div>
      <p className="text-ink-faint">
        This adds the {visionaries.team} team from its template, then opens New task for your theme or question.
      </p>
    </>
  );
}

// AskVisionariesBanners say what went wrong asking: the teams couldn't be
// read, policy.toml lacks the profiles the team needs (offering to add
// them), or adding the team failed.
export function AskVisionariesBanners({ visionaries }: { visionaries: AskVisionaries }) {
  return (
    <>
      {visionaries.loadError && <Banner title="Couldn't read the teams">{visionaries.loadError}</Banner>}
      {visionaries.missingProfiles.length > 0 && (
        <MissingProfilesOffer
          team={visionaries.team}
          profiles={visionaries.missingProfiles}
          adding={visionaries.adding}
          onAdd={visionaries.askAddingProfiles}
        />
      )}
      {visionaries.error && <Banner title="The Visionaries team isn't there">{visionaries.error}</Banner>}
    </>
  );
}

// TurnOnIdeas is Ideas' button on the Settings page, with what went wrong.
export function TurnOnIdeas() {
  const visionaries = useAskVisionaries();
  return (
    <div className="flex flex-col gap-3 text-[12px]">
      <AskVisionariesBanners visionaries={visionaries} />
      <div className="flex flex-col gap-2">
        <AskVisionariesOffer visionaries={visionaries} />
      </div>
    </div>
  );
}
