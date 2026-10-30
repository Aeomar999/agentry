// Package policyapi is the Policy page's HTTP API: the policy in force, the
// default profiles against yours, its history, and the changes the page
// makes to it, and a call tried as a profile would judge it. Web mounts its
// Routes as it mounts an extension's.
package policyapi

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// Source is the policy in force and any problem with the file on disk,
// and the one way to change it.
type Source interface {
	Current() *policy.Policy
	Problem() string
	Path() string
	Change(e policy.Edit, now time.Time) (*policy.Change, error)
}

// API holds what the policy's routes need: the log to record its changes
// in and read its versions from, the policy itself (nil for none: each
// route but the history is then a 404), the roster whose agents use its
// profiles, this Agentry's version, and where tasks' worktrees are kept.
type API struct {
	Log       extensions.Proposer
	Events    extensions.EventLog
	Policy    Source
	Roster    func() (agents.Roster, error)
	Version   string // as the comment above a profile it adds says
	Worktrees string // a tried call runs in one there when it names none
}

// Routes are the policy's endpoints, for the page alone: no agent's token
// changes the policy.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/policy", Auth: extensions.UI, Handler: a.showPolicy},
		{Pattern: "GET /api/policy/history", Auth: extensions.UI, Handler: a.policyHistory},
		{Pattern: "GET /api/policy/history/{position}", Auth: extensions.UI, Handler: a.policyVersion},
		{Pattern: "POST /api/policy/profiles/{name}/add", Auth: extensions.UI, Handler: a.addDefaultProfile},
		{Pattern: "POST /api/policy/profiles/{name}/update", Auth: extensions.UI, Handler: a.updateDefaultProfile},
		{Pattern: "POST /api/policy/profiles/{name}/use-default", Auth: extensions.UI, Handler: a.useDefaultProfile},
		{Pattern: "POST /api/policy/use-for-everyone/off", Auth: extensions.UI, Handler: a.turnOffUseForEveryone},
		{Pattern: "POST /api/policy/revert", Auth: extensions.UI, Handler: a.revertPolicy},
		{Pattern: "POST /api/policy/try", Auth: extensions.UI, Handler: a.tryPolicy},
	}
}

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

// EffectiveView is the profile that judges an agent's calls, with the text
// a card shows: "read-mostly, overridden by allow-everything".
type EffectiveView struct {
	policy.Effective
	Text string `json:"text"`
}

// EffectiveOf is the profile that judges the calls of an agent naming own.
func EffectiveOf(p *policy.Policy, own string) EffectiveView {
	e := p.EffectiveProfile(own)
	return EffectiveView{Effective: e, Text: e.String()}
}

// showPolicy is the policy in force, and why the file on disk isn't, if it isn't.
func (a API) showPolicy(w http.ResponseWriter, r *http.Request) {
	if a.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	current := a.Policy.Current()
	defaults, err := defaultsOf(current)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	users := map[string][]profileUser{}
	effective := map[string]EffectiveView{}
	for _, agent := range roster {
		own := current.EffectiveProfile(agent.Profile).Own
		users[own] = append(users[own], profileUser{Agent: agent.Name, FollowsDefault: agent.Profile == ""})
		effective[agent.Name] = EffectiveOf(current, agent.Profile)
	}
	for _, list := range users {
		slices.SortFunc(list, func(a, b profileUser) int { return cmp.Compare(a.Agent, b.Agent) })
	}
	httpapi.Reply(w, map[string]any{"path": a.Policy.Path(), "hash": current.Hash(), "problem": a.Policy.Problem(),
		"defaultProfile": current.DefaultProfile(), "useForEveryone": current.UseForEveryone(),
		"profiles": current.Summary(), "text": current.Text(), "version": a.Version,
		"defaults": defaults, "users": users, "effective": effective, "missing": current.MissingProfiles(AgentProfiles(roster))})
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

// seenHash is the version of the policy a change was made against.
type seenHash struct{ Hash string }

// addDefaultProfile appends a default profile your file lacks.
func (a API) addDefaultProfile(w http.ResponseWriter, r *http.Request) {
	a.changeProfile(w, r, "added profile %s", func(text []byte, name string) ([]byte, error) {
		return policy.AddProfile(text, name, a.Version)
	})
}

// updateDefaultProfile replaces an outdated profile you never changed with
// this version's.
func (a API) updateDefaultProfile(w http.ResponseWriter, r *http.Request) {
	a.changeProfile(w, r, "updated %s to the "+a.Version+" default", func(text []byte, name string) ([]byte, error) {
		return policy.UpdateProfile(text, name, a.Version)
	})
}

// useDefaultProfile replaces a profile you changed with this version's: your
// choice, after the page has shown you both. Keeping yours needs no call.
func (a API) useDefaultProfile(w http.ResponseWriter, r *http.Request) {
	a.changeProfile(w, r, "replaced your %s with the "+a.Version+" default", func(text []byte, name string) ([]byte, error) {
		return policy.UseDefaultProfile(text, name, a.Version)
	})
}

func (a API) changeProfile(w http.ResponseWriter, r *http.Request, change string, edit func(text []byte, name string) ([]byte, error)) {
	var request seenHash
	if !httpapi.Decode(w, r, &request) {
		return
	}
	name := r.PathValue("name")
	if _, ok := policy.ShippedProfile(name); !ok {
		http.Error(w, "Agentry ships no profile called "+name, http.StatusNotFound)
		return
	}
	a.changePolicy(w, request.Hash, fmt.Sprintf(change, name), func(text []byte) ([]byte, error) { return edit(text, name) })
}

// turnOffUseForEveryone comments out use_for_everyone, so each agent uses
// its own profile again.
func (a API) turnOffUseForEveryone(w http.ResponseWriter, r *http.Request) {
	var request seenHash
	if !httpapi.Decode(w, r, &request) {
		return
	}
	a.changePolicy(w, request.Hash, "turned off use_for_everyone", policy.TurnOffUseForEveryone)
}

// policyHistory is every version of the policy, newest first.
func (a API) policyHistory(w http.ResponseWriter, r *http.Request) {
	stream, err := a.Events.Stream(events.PolicyStream)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	httpapi.Reply(w, policy.History(stream))
}

// policyVersion is what reverting to the version at position would change:
// the diff from the text in force to that version's, and the profile each
// puts every agent under. A diff of "" is the version in force.
func (a API) policyVersion(w http.ResponseWriter, r *http.Request) {
	if a.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return
	}
	position, err := strconv.ParseInt(r.PathValue("position"), 10, 64)
	if err != nil {
		http.Error(w, "the position must be a number", http.StatusBadRequest)
		return
	}
	text, at, ok := a.versionAt(w, position)
	if !ok {
		return
	}
	useForEveryone, err := policy.UseForEveryoneOf([]byte(text))
	if err != nil {
		http.Error(w, "that version doesn't load: "+err.Error(), http.StatusConflict)
		return
	}
	current := a.Policy.Current()
	httpapi.Reply(w, map[string]any{"position": position, "at": at,
		"diff":           policy.LineDiff("in force", "version of "+versionTime(at), current.Text(), text),
		"useForEveryone": useForEveryone, "inForceUseForEveryone": current.UseForEveryone()})
}

// revertPolicy writes an earlier version back, as a new version.
func (a API) revertPolicy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Position int64
		Hash     string
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	text, at, ok := a.versionAt(w, request.Position)
	if !ok {
		return
	}
	change := "reverted to the version of " + versionTime(at)
	a.changePolicy(w, request.Hash, change, func(now []byte) ([]byte, error) {
		if string(now) == text {
			return nil, errors.New("that version is the one in force")
		}
		return []byte(text), nil
	})
}

// versionAt is the text of the version at position and when it was put in
// force. A position holding no version is a 404, a version whose text the
// log lacks a 409.
func (a API) versionAt(w http.ResponseWriter, position int64) (string, time.Time, bool) {
	stream, err := a.Events.Stream(events.PolicyStream)
	if err != nil {
		httpapi.Failed(w, err)
		return "", time.Time{}, false
	}
	text, at, err := policy.VersionAt(stream, position)
	switch {
	case errors.Is(err, policy.ErrNoVersion):
		http.Error(w, err.Error(), http.StatusNotFound)
		return "", time.Time{}, false
	case err != nil: // the version is there, but not its text
		http.Error(w, err.Error(), http.StatusConflict)
		return "", time.Time{}, false
	}
	return text, at, true
}

// versionTime is when a version was put in force, as a change names it.
func versionTime(at time.Time) string { return at.Local().Format("2 Jan 2006 15:04:05") }

// verdictView is a verdict as the page shows it.
type verdictView struct {
	Decision policy.Decision `json:"decision"`
	Profile  string          `json:"profile"`
	Rule     string          `json:"rule"`
	Why      string          `json:"why"`
	Policy   string          `json:"policy"`          // the hash of the version that decided
	Order    int             `json:"order,omitempty"` // the deciding rule's place in its profile, from 1
}

func verdictOf(v policy.Verdict) verdictView {
	return verdictView{Decision: v.Decision, Profile: v.Profile, Rule: v.Rule, Why: v.Why, Policy: v.Policy, Order: v.Order}
}

// tryPolicy judges a call as a profile would, once use_for_everyone is off,
// without running it or logging anything. When use_for_everyone puts
// another profile in its place, the reply says what that one decides today.
// With no workspace named, the call runs in a stand-in task's worktree.
func (a API) tryPolicy(w http.ResponseWriter, r *http.Request) {
	if a.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return
	}
	var request struct {
		Profile, Tool, Input, Workspace string
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	current := a.Policy.Current()
	switch {
	case !slices.Contains(current.ProfileNames(), request.Profile):
		http.Error(w, "policy.toml has no profile called "+request.Profile, http.StatusNotFound)
		return
	case request.Tool == "":
		http.Error(w, "name the tool the call uses", http.StatusBadRequest)
		return
	case request.Workspace != "" && !filepath.IsAbs(request.Workspace):
		http.Error(w, "the workspace must be an absolute path", http.StatusBadRequest)
		return
	case request.Workspace == "" && a.Worktrees == "":
		http.Error(w, "this server has no worktrees folder to put a stand-in workspace in: name an absolute workspace", http.StatusBadRequest)
		return
	}
	workspace := a.standInWorkspace()
	if request.Workspace != "" {
		workspace = filepath.Clean(request.Workspace)
	}
	call := policy.Call{Tool: request.Tool, Input: request.Input, Workspace: workspace}
	tried := map[string]any{"verdict": verdictOf(current.DecideAs(request.Profile, call)), "workspace": workspace}
	if everyone := current.UseForEveryone(); everyone != "" && everyone != request.Profile {
		tried["override"] = map[string]any{"profile": everyone, "verdict": verdictOf(current.Decide(request.Profile, call))}
	}
	httpapi.Reply(w, tried)
}

// standInWorkspace is the worktree a tried call runs in when it names none:
// one beside the real tasks', so the rules about the workspace judge it as
// they would a task's.
func (a API) standInWorkspace() string {
	return filepath.Join(a.Worktrees, "example")
}

// changePolicy makes edit to the policy file against the version seen,
// logs it as yours, and replies with the new version.
func (a API) changePolicy(w http.ResponseWriter, seen, change string, edit func(text []byte) ([]byte, error)) {
	if c, ok := a.Apply(w, seen, change, edit); ok {
		httpapi.Reply(w, map[string]any{"hash": c.Policy.Hash(), "change": change, "diff": c.Diff, "backup": c.Backup})
	}
}

// Apply is changePolicy without the reply, which web's teams use too. A
// refused change is a 409 that says why: the file changed since the page
// loaded it, the edit isn't one that can be made, or it would take a
// profile from agents that name it.
// Failing to read or write the file is a 500.
func (a API) Apply(w http.ResponseWriter, seen, change string, edit func(text []byte) ([]byte, error)) (*policy.Change, bool) {
	if a.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return nil, false
	}
	// The roster is read before Change takes the policy's lock, so an agent
	// created meanwhile could get a profile this change removes, unrefused.
	// With one person making both changes, that is accepted.
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return nil, false
	}
	var recorded error
	c, err := a.Policy.Change(policy.Edit{Seen: seen, Apply: edit,
		Check: func(next *policy.Policy) error {
			return a.Policy.Current().RefuseRemovingUsed(next, AgentProfiles(roster))
		},
		Record: func(c *policy.Change) {
			_, rejection, err := a.Log.Propose(policy.Loaded(a.Policy.Path(), c, change, httpapi.You))
			if rejection != nil {
				err = errors.New(rejection.Reason)
			}
			recorded = err
		},
	}, time.Now())
	var refusal *policy.Refusal
	switch {
	case errors.Is(err, policy.ErrStale):
		http.Error(w, "policy.toml changed since the page loaded it: try again", http.StatusConflict)
		return nil, false
	case errors.As(err, &refusal):
		http.Error(w, err.Error(), http.StatusConflict)
		return nil, false
	case err != nil:
		httpapi.Failed(w, err)
		return nil, false
	case recorded != nil:
		httpapi.Failed(w, fmt.Errorf("the policy changed, but recording the change failed: %w", recorded))
		return nil, false
	}
	return c, true
}

// AgentProfiles are the profiles the roster's agents name.
func AgentProfiles(roster agents.Roster) []policy.AgentProfile {
	var list []policy.AgentProfile
	for _, a := range roster {
		list = append(list, policy.AgentProfile{Agent: a.Name, Profile: a.Profile})
	}
	return list
}
