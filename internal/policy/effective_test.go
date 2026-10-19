package policy

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEffectiveProfile(t *testing.T) {
	everyone, err := Parse(Default(), home, protected)
	if err != nil {
		t.Fatal(err)
	}
	own := loadDefault(t)
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
	}
	for _, c := range cases {
		got := c.policy.EffectiveProfile(c.profile)
		if got != c.want || got.String() != c.shown {
			t.Errorf("%q under use_for_everyone %q: got %+v (%s), want %+v (%s)", c.profile, c.policy.UseForEveryone(), got, got, c.want, c.shown)
		}
		if verdict := c.policy.Decide(c.profile, bash("ls")); verdict.Profile != c.want.Profile {
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
