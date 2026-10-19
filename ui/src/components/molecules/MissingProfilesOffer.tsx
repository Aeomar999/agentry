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
  onAdd: () => void;
}

export function MissingProfilesOffer({ team, profiles, adding, disabled, onAdd }: MissingProfilesOfferProps) {
  return (
    <Banner tone="info" title={`The ${team} team needs ${profilesNamed(profiles)}, which your policy file doesn't have`}>
      <div className="flex flex-wrap items-center gap-3">
        <span className="min-w-0 flex-1">Agentry adds the default text to policy.toml, keeping a backup, then adds the {team} team.</span>
        <Button size="sm" variant="primary" disabled={adding || disabled} onClick={onAdd}>
          {adding ? "Adding…" : profiles.length === 1 ? "Add the profile and the team" : "Add the profiles and the team"}
        </Button>
      </div>
    </Banner>
  );
}
