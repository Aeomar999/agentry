package web

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
)

// The Policy page: what is in force, the default profiles against yours,
// who uses each profile, and the changes it makes for you. Each change
// goes through policy.Live.Change: made against the version the page saw,
// refused when it would take a profile from agents that name it, backed
// up, and logged as yours with its diff.

// defaultView is a profile Agentry ships, as your file has it.
type defaultView struct {
	policy.DefaultProfileStatus
	Text string `json:"text"`           // the profile as this version ships it
	Diff string `json:"diff,omitempty"` // yours against it, when outdated or edited
}

// profileUser is an agent with a profile. One that names none follows
// default_profile: it moves to the new default with it, so it doesn't stop
// the profile from being removed.
type profileUser struct {
	Agent          string `json:"agent"`
	FollowsDefault bool   `json:"followsDefault"`
}

// effectiveView is the profile that judges an agent's calls, with the text
// a card shows: "read-mostly, overridden by allow-everything".
type effectiveView struct {
	policy.Effective
	Text string `json:"text"`
}

func effectiveOf(p *policy.Policy, own string) effectiveView {
	e := p.EffectiveProfile(own)
	return effectiveView{Effective: e, Text: e.String()}
}

// showPolicy is the policy in force, and why the file on disk isn't, if it isn't.
func (s *Server) showPolicy(w http.ResponseWriter, r *http.Request) {
	if s.config.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	current := s.config.Policy.Current()
	defaults, err := defaultsOf(current)
	if err != nil {
		failed(w, err)
		return
	}
	users := map[string][]profileUser{}
	effective := map[string]effectiveView{}
	for _, a := range roster {
		own := current.EffectiveProfile(a.Profile).Own
		users[own] = append(users[own], profileUser{Agent: a.Name, FollowsDefault: a.Profile == ""})
		effective[a.Name] = effectiveOf(current, a.Profile)
	}
	for _, list := range users {
		slices.SortFunc(list, func(a, b profileUser) int { return cmp.Compare(a.Agent, b.Agent) })
	}
	reply(w, map[string]any{"path": s.config.Policy.Path(), "hash": current.Hash(), "problem": s.config.Policy.Problem(),
		"defaultProfile": current.DefaultProfile(), "useForEveryone": current.UseForEveryone(),
		"profiles": current.Summary(), "text": current.Text(), "version": s.version(),
		"defaults": defaults, "users": users, "effective": effective})
}

func defaultsOf(current *policy.Policy) ([]defaultView, error) {
	text := []byte(current.Text())
	statuses, err := policy.DefaultProfiles(text)
	if err != nil {
		return nil, err
	}
	views := []defaultView{}
	for _, status := range statuses {
		view := defaultView{DefaultProfileStatus: status}
		view.Text, _ = policy.ShippedProfile(status.Name)
		if status.State == policy.ProfileOutdated || status.State == policy.ProfileEdited {
			if view.Diff, err = policy.ProfileDiff(text, status.Name); err != nil {
				return nil, err
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// version is this Agentry's, as the comment above a profile it adds says.
func (s *Server) version() string {
	if s.config.Local.Version == "" {
		return "(unknown version)"
	}
	return s.config.Local.Version
}

// seenHash is the version of the policy a change was made against.
type seenHash struct{ Hash string }

// addDefaultProfile appends a default profile your file lacks.
func (s *Server) addDefaultProfile(w http.ResponseWriter, r *http.Request) {
	s.changeProfile(w, r, "added profile %s", func(text []byte, name string) ([]byte, error) {
		return policy.AddProfile(text, name, s.version())
	})
}

// updateDefaultProfile replaces an outdated profile you never changed with
// this version's.
func (s *Server) updateDefaultProfile(w http.ResponseWriter, r *http.Request) {
	s.changeProfile(w, r, "updated %s to the "+s.version()+" default", func(text []byte, name string) ([]byte, error) {
		return policy.UpdateProfile(text, name, s.version())
	})
}

// useDefaultProfile replaces a profile you changed with this version's: your
// choice, after the page has shown you both. Keeping yours needs no call.
func (s *Server) useDefaultProfile(w http.ResponseWriter, r *http.Request) {
	s.changeProfile(w, r, "replaced your %s with the "+s.version()+" default", func(text []byte, name string) ([]byte, error) {
		return policy.UseDefaultProfile(text, name, s.version())
	})
}

func (s *Server) changeProfile(w http.ResponseWriter, r *http.Request, change string, edit func(text []byte, name string) ([]byte, error)) {
	var request seenHash
	if !decode(w, r, &request) {
		return
	}
	name := r.PathValue("name")
	if _, ok := policy.ShippedProfile(name); !ok {
		http.Error(w, "Agentry ships no profile called "+name, http.StatusNotFound)
		return
	}
	s.changePolicy(w, request.Hash, fmt.Sprintf(change, name), func(text []byte) ([]byte, error) { return edit(text, name) })
}

// turnOffUseForEveryone comments out use_for_everyone, so each agent uses
// its own profile again.
func (s *Server) turnOffUseForEveryone(w http.ResponseWriter, r *http.Request) {
	var request seenHash
	if !decode(w, r, &request) {
		return
	}
	s.changePolicy(w, request.Hash, "turned off use_for_everyone", policy.TurnOffUseForEveryone)
}

// policyHistory is every version of the policy, newest first.
func (s *Server) policyHistory(w http.ResponseWriter, r *http.Request) {
	stream, err := s.store.Stream(events.PolicyStream)
	if err != nil {
		failed(w, err)
		return
	}
	reply(w, policy.History(stream))
}

// revertPolicy writes an earlier version back, as a new version.
func (s *Server) revertPolicy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Position int64
		Hash     string
	}
	if !decode(w, r, &request) {
		return
	}
	stream, err := s.store.Stream(events.PolicyStream)
	if err != nil {
		failed(w, err)
		return
	}
	text, at, err := policy.VersionAt(stream, request.Position)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	change := "reverted to the version of " + at.Local().Format("2 Jan 2006 15:04:05")
	s.changePolicy(w, request.Hash, change, func(now []byte) ([]byte, error) {
		if string(now) == text {
			return nil, errors.New("that version is the one in force")
		}
		return []byte(text), nil
	})
}

// changePolicy makes edit to the policy file against the version seen,
// logs it as yours, and replies with the new version.
func (s *Server) changePolicy(w http.ResponseWriter, seen, change string, edit func(text []byte) ([]byte, error)) {
	if c, ok := s.applyPolicy(w, seen, change, edit); ok {
		reply(w, map[string]any{"hash": c.Policy.Hash(), "change": change, "diff": c.Diff, "backup": c.Backup})
	}
}

// applyPolicy is changePolicy without the reply. A refused change is a 409
// that says why: the file changed since the page loaded it, the edit isn't
// one that can be made, or it would take a profile from agents that name it.
func (s *Server) applyPolicy(w http.ResponseWriter, seen, change string, edit func(text []byte) ([]byte, error)) (*policy.Change, bool) {
	if s.config.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return nil, false
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return nil, false
	}
	var recorded error
	c, err := s.config.Policy.Change(policy.Edit{Seen: seen, Apply: edit,
		Check: func(next *policy.Policy) error {
			return s.config.Policy.Current().RefuseRemovingUsed(next, agentProfiles(roster))
		},
		Record: func(c *policy.Change) {
			_, rejection, err := s.store.Propose(policy.Loaded(s.config.Policy.Path(), c, change, you))
			if rejection != nil {
				err = errors.New(rejection.Reason)
			}
			recorded = err
		},
	}, time.Now())
	switch {
	case errors.Is(err, policy.ErrStale):
		http.Error(w, "policy.toml changed since the page loaded it: reload the page and try again", http.StatusConflict)
		return nil, false
	case err != nil:
		http.Error(w, err.Error(), http.StatusConflict)
		return nil, false
	case recorded != nil:
		failed(w, fmt.Errorf("the policy changed, but recording the change failed: %w", recorded))
		return nil, false
	}
	return c, true
}

func agentProfiles(roster agents.Roster) []policy.AgentProfile {
	var list []policy.AgentProfile
	for _, a := range roster {
		list = append(list, policy.AgentProfile{Agent: a.Name, Profile: a.Profile})
	}
	return list
}
