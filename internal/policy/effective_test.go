package policy

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEffectiveProfile(t *testing.T) {
	everyone, err := Parse(Default(), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	own := loadDefault(t)
	lacking, err := Parse([]byte(cutProfile(t, own.Text(), "read-mostly")), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	everyoneLacking, err := Parse([]byte(cutProfile(t, everyone.Text(), "read-mostly")), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		policy  *Policy
		profile string
		want    Effective
		shown   string
	}{
		{own, "read-mostly", Effective{Profile: "read-mostly", Own: "read-mostly"}, "read-mostly"},
		{own, "", Effective{Profile: "developer", Own: "developer"}, "developer"},
		{everyone, "read-mostly", Effective{Profile: "allow-everything", Own: "read-mostly", Overridden: true}, "read-mostly, overridden by allow-everything"},
		{everyone, "", Effective{Profile: "allow-everything", Own: "developer", Overridden: true}, "developer, overridden by allow-everything"},
		{everyone, "allow-everything", Effective{Profile: "allow-everything", Own: "allow-everything"}, "allow-everything"},
		{lacking, "read-mostly", Effective{Profile: "read-mostly", Own: "read-mostly", Missing: true}, "read-mostly (missing from policy.toml)"},
		{everyoneLacking, "read-mostly", Effective{Profile: "allow-everything", Own: "read-mostly", Overridden: true, Missing: true},
			"read-mostly (missing from policy.toml), overridden by allow-everything"},
	}
	for _, c := range cases {
		got := c.policy.EffectiveProfile(c.profile)
		if got != c.want || got.String() != c.shown {
			t.Errorf("%q under use_for_everyone %q: got %+v (%s), want %+v (%s)", c.profile, c.policy.UseForEveryone(), got, got, c.want, c.shown)
		}
		verdict := c.policy.Decide(c.profile, bash("ls"))
		if c.want.Missing && !c.want.Overridden {
			if verdict.Decision != Deny || verdict.Rule != "profile read-mostly isn't in policy.toml" {
				t.Errorf("%q is missing, so every call is denied: got %+v", c.profile, verdict)
			}
		} else if verdict.Profile != c.want.Profile {
			t.Errorf("%q: Decide used %s, not the effective %s", c.profile, verdict.Profile, c.want.Profile)
		}
	}
}

var team = []AgentProfile{{"reviewer", "read-only"}, {"dreamer", "read-mostly"}, {"engineer", ""}, {"scout", "read-mostly"}}

func TestProfileUsersCountsEachAgentsOwnProfile(t *testing.T) {
	everyone, _ := Parse(Default(), home, protected)
	want := map[string][]string{"read-only": {"reviewer"}, "read-mostly": {"dreamer", "scout"}, "developer": {"engineer"}}
	if got := everyone.ProfileUsers(team); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMissingProfilesNamesTheAgentsThatNeedThem(t *testing.T) {
	withoutReadMostly, err := Parse([]byte(cutProfile(t, loadDefault(t).Text(), "read-mostly")), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	stranger := append(slices.Clone(team), AgentProfile{"ghost", "haunted"})
	want := map[string][]string{"read-mostly": {"dreamer", "scout"}, "haunted": {"ghost"}}
	if got := withoutReadMostly.MissingProfiles(stranger); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := loadDefault(t).MissingProfiles(team); len(got) != 0 {
		t.Errorf("the default policy has every profile the team uses, got %v", got)
	}
}

func TestRemovingAProfileInUseIsRefused(t *testing.T) {
	before := loadDefault(t)
	withoutReadMostly := cutProfile(t, before.Text(), "read-mostly")
	after, err := Parse([]byte(withoutReadMostly), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	err = before.RefuseRemovingUsed(after, team)
	var inUse *ProfileInUseError
	if !errors.As(err, &inUse) || inUse.Profile != "read-mostly" || !reflect.DeepEqual(inUse.Agents, []string{"dreamer", "scout"}) {
		t.Fatalf("got %v, want read-mostly refused for dreamer and scout", err)
	}
	if want := `profile "read-mostly" is still used by dreamer, scout`; !strings.Contains(err.Error(), want) {
		t.Errorf("the error %q should say %q", err, want)
	}

	if err := before.RefuseRemovingUsed(after, team[:1]); err != nil {
		t.Errorf("no agent uses read-mostly: %v", err)
	}
	if err := after.RefuseRemovingUsed(after, team); err != nil {
		t.Errorf("read-mostly was missing already, so this change doesn't remove it: %v", err)
	}
	withoutAutonomous, _ := Parse([]byte(cutProfile(t, before.Text(), "autonomous")), home, protected)
	if err := before.RefuseRemovingUsed(withoutAutonomous, team); err != nil {
		t.Errorf("no agent uses autonomous: %v", err)
	}
}

func cutProfile(t *testing.T, text, name string) string {
	t.Helper()
	b, ok, err := findBlock([]byte(text), name)
	if err != nil || !ok {
		t.Fatalf("no block for %s: %v", name, err)
	}
	return text[:b.start] + text[b.end:]
}

func TestDecideAsSkipsUseForEveryone(t *testing.T) {
	everyone, err := Parse(Default(), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	push := bash("git push origin feature")
	if got := everyone.Decide("developer", push); got.Decision != Allow || got.Profile != "allow-everything" {
		t.Errorf("Decide follows use_for_everyone: got %+v", got)
	}
	got := everyone.DecideAs("developer", push)
	if got.Decision != Ask || got.Profile != "developer" || got.Rule != "every push leaves this machine: you approve it" || got.Policy != everyone.Hash() {
		t.Errorf("DecideAs judges as developer: got %+v", got)
	}
	if got := everyone.DecideAs("haunted", push); got.Decision != Deny || got.Rule != "profile haunted isn't in policy.toml" {
		t.Errorf("an unknown profile denies: got %+v", got)
	}
	for _, profile := range []string{"developer", "allow-everything"} {
		if got := everyone.DecideAs(profile, file("Read", "/home/dev/.agentry/policy.toml")); got.Decision != Deny || got.Rule != "Agentry's own files are off limits" {
			t.Errorf("Agentry's own files stay off limits for %q: got %+v", profile, got)
		}
	}
}
