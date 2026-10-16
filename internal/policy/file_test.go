package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimal = `
version = 1
shell_tools = ["Bash"]
default_profile = "p"
[profiles.p]
otherwise = "ask"
`

// A typo must never open the gate: anything Agentry doesn't understand is refused.
func TestABadPolicyIsRefusedWithTheReason(t *testing.T) {
	cases := map[string]struct{ text, want string }{
		"not TOML":              {"version = ", "line 1, column"},
		"wrong version":         {strings.Replace(minimal, "version = 1", "version = 2", 1), "version must be 1"},
		"misspelt key":          {minimal + "otherwse = \"allow\"\n", "otherwse"},
		"unknown decision":      {strings.Replace(minimal, `otherwise = "ask"`, `otherwise = "maybe"`, 1), `otherwise must be "allow", "ask" or "deny"`},
		"missing default":       {strings.Replace(minimal, `default_profile = "p"`, ``, 1), "default_profile is missing"},
		"undefined default":     {strings.Replace(minimal, `default_profile = "p"`, `default_profile = "q"`, 1), `profile "q" is named but not defined`},
		"rule without a name":   {minimal + "[[profiles.p.rules]]\ndecision = \"allow\"\n", "a rule needs a name"},
		"rule with a bad where": {minimal + "[[profiles.p.rules]]\nname = \"x\"\ndecision = \"allow\"\nwhere = \"near\"\n", `where must be "workspace" or "outside"`},
		"rule misspelt field":   {minimal + "[[profiles.p.rules]]\nname = \"x\"\ndecision = \"allow\"\nrun_only = ['ls']\n", "run_only"},
	}
	for name, c := range cases {
		_, err := Parse([]byte(c.text), home, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", name, err, c.want)
		}
	}
	if _, err := Parse([]byte(minimal), home, nil); err != nil {
		t.Errorf("the minimal policy: %v", err)
	}
}

func TestEnsureFileWritesTheDefaultOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	if err := EnsureFile(path); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(minimal), 0o600) // the user's edit
	if err := EnsureFile(path); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path, home, nil)
	if err != nil || len(p.ProfileNames()) != 1 {
		t.Errorf("the user's file must be kept: %v %v", p, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}

func TestTheDefaultHasTheFourProfiles(t *testing.T) {
	names := strings.Join(loadDefault(t).ProfileNames(), " ")
	if names != "allow-everything autonomous developer read-only" {
		t.Errorf("profiles: %s", names)
	}
}

func TestLiveReloadsAValidFileAndKeepsThePolicyOverABrokenOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	live, err := OpenLive(path, home, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded, refused := live.Reload(); loaded != nil || refused != nil {
		t.Errorf("an unchanged file reports nothing: %v %v", loaded, refused)
	}
	first := live.Current().Hash()

	os.WriteFile(path, []byte(minimal), 0o600)
	if loaded, _ := live.Reload(); loaded == nil || live.Current().Hash() == first || live.Problem() != "" {
		t.Fatalf("a valid change replaces the policy")
	}
	working := live.Current().Hash()

	os.WriteFile(path, []byte(strings.Replace(minimal, `"ask"`, `"maybe"`, 1)), 0o600)
	if _, refused := live.Reload(); refused == nil || live.Current().Hash() != working || !strings.Contains(live.Problem(), "maybe") {
		t.Errorf("a broken file must be refused and the working policy kept: %v, problem %q", refused, live.Problem())
	}
	if _, refused := live.Reload(); refused != nil {
		t.Errorf("the same broken file is reported once, got %v again", refused)
	}
	if got := live.Decide("p", Call{Tool: "Bash", Input: "ls", Workspace: "/w"}); got.Decision != Ask || got.Policy != working {
		t.Errorf("decisions use the working policy: %+v", got)
	}

	os.WriteFile(path, []byte(minimal+"# fixed\n"), 0o600)
	if loaded, _ := live.Reload(); loaded == nil || live.Problem() != "" {
		t.Errorf("fixing the file clears the problem: %q", live.Problem())
	}
}

func TestABrokenFileStopsTheFirstStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte("version = "), 0o600)
	if _, err := OpenLive(path, home, nil); err == nil {
		t.Error("Agentry must not run without a policy")
	}
}
