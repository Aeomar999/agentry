package policy

import (
	"errors"
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
		"misspelt redirect":     {minimal + "[[profiles.p.rules]]\nname = \"x\"\ndecision = \"deny\"\nredirect_to_file = true\n", "redirect_to_file"},
		"redirect not a bool":   {minimal + "[[profiles.p.rules]]\nname = \"x\"\ndecision = \"deny\"\nredirects_to_file = \"yes\"\n", "RedirectsToFile"},
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

func TestTheDefaultHasTheSixProfiles(t *testing.T) {
	names := strings.Join(loadDefault(t).ProfileNames(), " ")
	if names != "allow-everything autonomous developer network-diagnosis read-mostly read-only" {
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

func TestABrokenFileAtStartNamesTheLineAndTheNewestBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte("version = 1\n\n[profile.p\n"), 0o600)
	older, newer := path+"-backup-20260101T000000.000000000Z", path+"-backup-20260102T000000.000000000Z"
	for _, name := range []string{newer, older, path + "-mine"} {
		os.WriteFile(name, []byte(minimal), 0o600)
	}
	os.Mkdir(path+"-backup-99999999T999999.999999999Z", 0o700)

	_, err := OpenLive(path, home, nil)
	if err == nil {
		t.Fatal("Agentry must not run without a policy")
	}
	for _, want := range []string{"policy " + path + " is refused: ", "line 3", "cp '" + newer + "' '" + path + "'"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error says %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), older) {
		t.Errorf("only the newest backup is named:\n%v", err)
	}
}

// An error that quotes the file starts on a line of its own, and so does
// its hint.
func TestAnUnknownKeyAtStartPutsTheHintOnItsOwnLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte(strings.Replace(minimal, "[profiles.p]", "bogus_key = true\n[profiles.p]", 1)), 0o600)
	backup := path + "-backup-20260101T000000.000000000Z"
	os.WriteFile(backup, []byte(minimal), 0o600)

	_, err := OpenLive(path, home, nil)
	if err == nil {
		t.Fatal("Agentry must not run without a policy")
	}
	lines := strings.Split(err.Error(), "\n")
	if !strings.HasSuffix(lines[0], " is refused:") || len(lines) < 3 || lines[1] != "line 5, column 1: unknown key bogus_key" ||
		!strings.HasPrefix(lines[2], "2| ") {
		t.Errorf("the error starts a line of its own, with where the key is, before the file's first quoted line:\n%v", err)
	}
	hint := "To go back to its newest backup, from before Agentry's last change: cp '" + backup + "' '" + path + "'"
	if len(lines) < 2 || lines[len(lines)-1] != hint || lines[len(lines)-2] != `7| otherwise = "ask"` {
		t.Errorf("the hint is the last line and the file's last quoted line is before it:\n%v", err)
	}
}

// A ' in the path must not break the cp line.
func TestTheStartHintQuotesAPathWithAQuote(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's")
	os.Mkdir(dir, 0o700)
	path := filepath.Join(dir, "policy.toml")
	os.WriteFile(path, []byte("version = "), 0o600)
	os.WriteFile(path+"-backup-20260101T000000.000000000Z", []byte(minimal), 0o600)

	_, err := OpenLive(path, home, nil)
	quoted := strings.ReplaceAll(path, "'", `'\''`)
	if want := "cp '" + quoted + "-backup-20260101T000000.000000000Z' '" + quoted + "'"; err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("the error ends %q:\n%v", want, err)
	}
}

func TestARefusedFileAtStartSaysThereIsNoBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte(strings.Replace(minimal, "version = 1", "version = 2", 1)), 0o600)
	_, err := OpenLive(path, home, nil)
	if err == nil || !strings.HasSuffix(err.Error(), "; there is no backup of it") || !strings.Contains(err.Error(), path) {
		t.Errorf("the error names the file and says there is no backup: %v", err)
	}
}

func TestTheStartErrorWrapsWhyTheFileIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte("version = "), 0o600)
	_, want := Parse([]byte("version = "), home, nil)
	_, err := OpenLive(path, home, nil)
	if err == nil || errors.Unwrap(err) == nil || errors.Unwrap(err).Error() != want.Error() {
		t.Errorf("OpenLive wraps Parse's error %q: %v", want, err)
	}
}

func TestUseForEveryoneIsReadFromTheTextAlone(t *testing.T) {
	cases := map[string]struct{ text, want string }{
		"set":           {strings.Replace(minimal, `default_profile = "p"`, `default_profile = "p"`+"\nuse_for_everyone = \"p\"", 1), "p"},
		"unset":         {minimal, ""},
		"commented out": {strings.Replace(minimal, `default_profile = "p"`, `default_profile = "p"`+"\n# use_for_everyone = \"p\"", 1), ""},
		// Read, not checked: a profile it names but doesn't define is still what it says.
		"undefined profile": {strings.Replace(minimal, `default_profile = "p"`, `default_profile = "p"`+"\nuse_for_everyone = \"q\"", 1), "q"},
	}
	for name, c := range cases {
		if got, err := UseForEveryoneOf([]byte(c.text)); err != nil || got != c.want {
			t.Errorf("%s: got %q, %v, want %q", name, got, err, c.want)
		}
	}
	if _, err := UseForEveryoneOf([]byte("use_for_everyone = ")); err == nil || !strings.Contains(err.Error(), "line 1, column") {
		t.Errorf("bad TOML: %v", err)
	}
}
