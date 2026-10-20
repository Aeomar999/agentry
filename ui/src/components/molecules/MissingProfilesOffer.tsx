import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { profilesNamed } from "@/lib/teams";

// MissingProfilesOffer offers to add a team that was refused because
// policy.toml lacks the default profiles it needs, adding those first.
interface MissingProfilesOfferProps {
  team: string;
  profiles: string[];
  adding: boolean; // this offer's add is under way
  disabled?: boolean;
  // The team is one you built, so the offer creates it rather than adds
  // it, or one you edited, whose changes it saves.
  then?: "add" | "create" | "save";
  onAdd: () => void;
}

export function MissingProfilesOffer({ team, profiles, adding, disabled, then = "add", onAdd }: MissingProfilesOfferProps) {
  const theProfiles = profiles.length === 1 ? "the profile" : "the profiles";
  const after = { add: `adds the ${team} team`, create: `creates the ${team} team`, save: "saves your changes" }[then];
  const button = `Add ${theProfiles} and ${{ add: "the team", create: "create the team", save: "save the team" }[then]}`;
  return (
    <Banner tone="info" title={`The ${team} team needs ${profilesNamed(profiles)}, which your policy file doesn't have`}>
      <div className="flex flex-col items-start gap-2">
        <span>Agentry adds the default text to policy.toml, keeping a backup, then {after}.</span>
        <Button size="sm" variant="primary" disabled={adding || disabled} onClick={onAdd}>
          {adding ? "Adding…" : button}
        </Button>
      </div>
    </Banner>
  );
}
