package agents

import (
	"regexp"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/policy"
)

// The default prompts tell agents to run commands. None of them may be one
// the policy denies, or an agent following its own prompt gets blocked.
func TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits(t *testing.T) {
	// The developer profile's own rules, without the shipped use_for_everyone.
	rules := strings.Replace(string(policy.Default()), "\nuse_for_everyone = ", "\n# use_for_everyone = ", 1)
	shipped, err := policy.Parse([]byte(rules), "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile("`(git [^`]+)`")
	pushes := 0
	for _, agent := range FromEvents(seeded()) {
		for _, match := range quoted.FindAllStringSubmatch(agent.Prompt, -1) {
			for _, command := range strings.Split(match[1], " then ") {
				verdict := shipped.Decide("developer", policy.Call{Tool: "Bash", Input: command, Workspace: "/w"})
				if verdict.Decision == policy.Deny {
					t.Errorf("%s's prompt says to run %q, which is denied: %s", agent.Name, command, verdict.Rule)
				}
				if strings.HasPrefix(command, "git push") {
					pushes++
					if verdict.Decision != policy.Ask {
						t.Errorf("%s's push %q must ask you, got %s", agent.Name, command, verdict.Decision)
					}
				}
			}
		}
	}
	if pushes == 0 {
		t.Error("the deployer's prompt should say how it pushes")
	}
}

// The Visionaries read the board to ground their ideas, and never change
// the code or commit it.
func TestTheVisionariesReadTheBoardAndChangeNothing(t *testing.T) {
	rules := strings.Replace(string(policy.Default()), "\nuse_for_everyone = ", "\n# use_for_everyone = ", 1)
	shipped, err := policy.Parse([]byte(rules), "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	visionaries, _ := TemplateNamed("Visionaries")
	for _, agent := range visionaries.Agents {
		for _, c := range []struct {
			call policy.Call
			want policy.Decision
		}{
			{policy.Call{Tool: "mcp__agentry__list_tasks", Input: "{}", Workspace: "/w"}, policy.Allow},
			{policy.Call{Tool: "mcp__agentry__show_task", Input: `{"task":"task-1"}`, Workspace: "/w"}, policy.Allow},
			{policy.Call{Tool: "mcp__agentry__create_task", Input: "{}", Workspace: "/w"}, policy.Ask},
			{policy.Call{Tool: "Bash", Input: "git log --oneline -30", Workspace: "/w"}, policy.Allow},
			{policy.Call{Tool: "Edit", Input: "/w/main.go", Workspace: "/w"}, policy.Deny},
			{policy.Call{Tool: "Write", Input: "/w/ideas.md", Workspace: "/w"}, policy.Deny},
			{policy.Call{Tool: "Bash", Input: "git commit -am 'ideas'", Workspace: "/w"}, policy.Deny},
		} {
			if got := shipped.Decide(agent.Profile, c.call); got.Decision != c.want {
				t.Errorf("%s (%s) / %s %s: got %s (%s), want %s", agent.Name, agent.Profile, c.call.Tool, c.call.Input, got.Decision, got.Reason(), c.want)
			}
		}
	}
}
