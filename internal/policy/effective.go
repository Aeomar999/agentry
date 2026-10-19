package policy

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Effective is the profile that judges an agent's calls.
type Effective struct {
	Profile    string `json:"profile"`    // the profile in force
	Own        string `json:"own"`        // the agent's own profile, default_profile if it names none
	Overridden bool   `json:"overridden"` // use_for_everyone puts Profile in place of Own
}

// String is how a card shows it: "read-mostly, overridden by allow-everything".
func (e Effective) String() string {
	if e.Overridden {
		return e.Own + ", overridden by " + e.Profile
	}
	return e.Profile
}

// EffectiveProfile is the profile in force for an agent whose own profile
// is own ("" for the default).
func (p *Policy) EffectiveProfile(own string) Effective {
	if own == "" {
		own = p.defaultProfile
	}
	if p.useForEveryone != "" && p.useForEveryone != own {
		return Effective{Profile: p.useForEveryone, Own: own, Overridden: true}
	}
	return Effective{Profile: own, Own: own}
}

// AgentProfile is an agent and the profile it names ("" for the default).
type AgentProfile struct {
	Agent, Profile string
}

// ProfileUsers lists, for each profile agents have as their own, those
// agents, sorted. use_for_everyone doesn't count: turned off, it gives
// each agent its own profile back.
func (p *Policy) ProfileUsers(agents []AgentProfile) map[string][]string {
	users := map[string][]string{}
	for _, a := range agents {
		own := p.EffectiveProfile(a.Profile).Own
		users[own] = append(users[own], a.Agent)
	}
	for _, names := range users {
		slices.Sort(names)
	}
	return users
}

// ProfileInUseError refuses a policy that drops a profile agents still have.
type ProfileInUseError struct {
	Profile string
	Agents  []string
}

func (e *ProfileInUseError) Error() string {
	return fmt.Sprintf("profile %q is still used by %s: give them another profile first", e.Profile, strings.Join(e.Agents, ", "))
}

// RefuseRemovingUsed refuses next, the policy that would replace p, when it
// drops a profile of p's that agents name, with a ProfileInUseError for
// each. Agents that name none move to next's default_profile, which next
// always has, and a profile p already lacks isn't next's doing.
func (p *Policy) RefuseRemovingUsed(next *Policy, agents []AgentProfile) error {
	var refusals []error
	for _, name := range p.ProfileNames() {
		var users []string
		for _, a := range agents {
			if a.Profile == name {
				users = append(users, a.Agent)
			}
		}
		if _, kept := next.profiles[name]; !kept && len(users) > 0 {
			slices.Sort(users)
			refusals = append(refusals, &ProfileInUseError{Profile: name, Agents: users})
		}
	}
	return errors.Join(refusals...)
}
