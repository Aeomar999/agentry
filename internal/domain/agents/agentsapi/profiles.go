package agentsapi

import (
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
)

// unshippedProfile says which agent needs a missing profile that Agentry
// doesn't ship, so only you can write it, or "".
func unshippedProfile(members []agents.Agent, missing []string) string {
	for _, a := range members {
		profile := strings.TrimSpace(a.Profile)
		if _, shipped := policy.ShippedProfile(profile); slices.Contains(missing, profile) && !shipped {
			return strings.TrimSpace(a.Name) + "'s profile " + profile + " isn't in policy.toml, and Agentry doesn't ship it: write it in policy.toml, or pick another profile"
		}
	}
	return ""
}

// hasProfiles says whether the policy has every profile the team needs,
// adding the missing ones when asked; when it says no, it has replied.
func (a API) hasProfiles(w http.ResponseWriter, team string, missing []string, addMissing bool, hash string) bool {
	if len(missing) == 0 {
		return true
	}
	if addMissing {
		return a.addProfilesFor(w, team, missing, hash)
	}
	replyWith(w, http.StatusConflict, map[string]any{"missingProfiles": missing,
		"error": "the " + team + " team needs " + profilesNamed(missing) + ", which policy.toml doesn't have: add it on the Policy page"})
	return false
}

// missingProfiles are the profiles these agents of a team need that the
// policy lacks, in the agents' order.
func (a API) missingProfiles(members []agents.Agent) []string {
	var missing []string
	for _, member := range members {
		if profile := strings.TrimSpace(member.Profile); a.unknownProfile(profile) != "" && !slices.Contains(missing, profile) {
			missing = append(missing, profile)
		}
	}
	return missing
}

// addProfilesFor adds the default profiles a team needs in one change of
// the policy, or replies why it can't: a profile Agentry doesn't ship is
// yours to write.
func (a API) addProfilesFor(w http.ResponseWriter, team string, missing []string, seen string) bool {
	for _, name := range missing {
		if _, shipped := policy.ShippedProfile(name); !shipped {
			http.Error(w, "the "+team+" team needs profile "+name+", which Agentry doesn't ship: write it in policy.toml yourself, then add the team", http.StatusConflict)
			return false
		}
	}
	// "" means the policy in force; Live's text check still refuses a hand edit that isn't reloaded.
	if seen == "" {
		seen = a.Policy.Current().Hash()
	}
	_, ok := a.ApplyPolicy(w, seen, "added "+profilesNamed(missing)+" for team "+team, func(text []byte) ([]byte, error) {
		var err error
		for _, name := range missing {
			if text, err = policy.AddProfile(text, name, a.Version); err != nil {
				return nil, err
			}
		}
		return text, nil
	})
	return ok
}

// profilesNamed is "profile read-mostly", or "profiles a, b".
func profilesNamed(names []string) string {
	if len(names) == 1 {
		return "profile " + names[0]
	}
	return "profiles " + strings.Join(names, ", ")
}

// unknownProfile says why an agent can't have profile, or "" when the policy
// has it. A policy written before a profile was added doesn't have it.
func (a API) unknownProfile(profile string) string {
	if profile == "" || a.Policy == nil || slices.Contains(a.Policy.Current().ProfileNames(), profile) {
		return ""
	}
	if _, shipped := policy.ShippedProfile(profile); shipped {
		return "policy.toml has no profile called " + profile + ": Agentry ships one, so add it on the Policy page, then try again"
	}
	return "policy.toml has no profile called " + profile + ": write it in policy.toml, or pick another profile"
}
