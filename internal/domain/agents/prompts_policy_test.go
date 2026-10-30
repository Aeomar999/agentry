package agents

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
)

// The default prompts tell agents to run commands. None of them may be one
// the agent's own profile denies, or an agent following its prompt gets
// blocked. Every template's agents are checked.
func TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits(t *testing.T) {
	// The profiles' own rules, without the shipped use_for_everyone.
	rules := strings.Replace(string(policy.Default()), "\nuse_for_everyone = ", "\n# use_for_everyone = ", 1)
	shipped, err := policy.Parse([]byte(rules), "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile("`((?:git|gh|glab) [^`]+)`")
	pushes, pipelines := 0, 0
	for _, template := range Templates {
		for _, agent := range template.Prompted() {
			for _, match := range quoted.FindAllStringSubmatch(agent.Prompt, -1) {
				for _, command := range strings.Split(match[1], " then ") {
					command = strings.ReplaceAll(command, "<hash>", "60c4a34dc947614f669110a4621eafe214f1527a")
					verdict := shipped.Decide(agent.Profile, policy.Call{Tool: "Bash", Input: command, Workspace: "/w"})
					if verdict.Decision == policy.Deny {
						t.Errorf("%s's prompt (%s) says to run %q, which is denied: %s", agent.Name, agent.Profile, command, verdict.Rule)
					}
					if strings.HasPrefix(command, "git push") {
						pushes++
						if verdict.Decision != policy.Ask {
							t.Errorf("%s's push %q must ask you, got %s", agent.Name, command, verdict.Decision)
						}
					}
					if strings.HasPrefix(command, "gh ") || strings.HasPrefix(command, "glab ") {
						pipelines++
						if verdict.Decision != policy.Allow {
							t.Errorf("%s's pipeline report %q must not wait for you, got %s (%s)", agent.Name, command, verdict.Decision, verdict.Reason())
						}
					}
				}
			}
		}
	}
	if pushes == 0 {
		t.Error("the deployer's prompt should say how it pushes")
	}
	if pipelines == 0 {
		t.Error("the deployer's prompt should say how it reports the pipeline")
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

// An agent whose profile isn't in the policy is denied every call, so every
// template may only name profiles Agentry ships.
func TestEveryTemplateProfileIsInTheShippedPolicy(t *testing.T) {
	shipped, err := policy.Parse(policy.Default(), "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range Templates {
		for _, agent := range template.Agents {
			if !slices.Contains(shipped.ProfileNames(), agent.Profile) {
				t.Errorf("%s/%s: profile %q is not in the shipped policy", template.Team, agent.Name, agent.Profile)
			}
		}
	}
}

// The Networking checkers run the diagnosis commands their prompts name
// without asking, and a live change asks whoever runs it.
func TestTheNetworkingTeamDiagnosesAndAsksBeforeChanging(t *testing.T) {
	rules := strings.Replace(string(policy.Default()), "\nuse_for_everyone = ", "\n# use_for_everyone = ", 1)
	shipped, err := policy.Parse([]byte(rules), "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	networking, _ := TemplateNamed("Networking")
	quoted := regexp.MustCompile("`((?:ping|traceroute|mtr|dig|ss|ip|docker|ufw|nft|iptables|wg) [^`]+)`")
	placeholders := strings.NewReplacer("<host>", "example.com", "<name>", "example.com", "<server>", "1.1.1.1", "<network>", "ingress")
	var diagnoses []string
	for _, agent := range networking.Prompted() {
		if agent.Column != Reviews {
			continue
		}
		for _, match := range quoted.FindAllStringSubmatch(agent.Prompt, -1) {
			diagnoses = append(diagnoses, placeholders.Replace(match[1]))
		}
	}
	if len(diagnoses) < 10 {
		t.Fatalf("the checkers' prompts should name their diagnosis commands, found %v", diagnoses)
	}
	changes := []string{"ufw allow 443", "ip route add 10.9.0.0/24 via 192.168.1.1", "systemctl restart nginx"}
	for _, agent := range networking.Agents {
		for _, command := range diagnoses {
			verdict := shipped.Decide(agent.Profile, policy.Call{Tool: "Bash", Input: command, Workspace: "/w"})
			if agent.Column == Reviews && verdict.Decision != policy.Allow || verdict.Decision == policy.Deny {
				t.Errorf("%s (%s) / %q: got %s (%s)", agent.Name, agent.Profile, command, verdict.Decision, verdict.Reason())
			}
		}
		for _, command := range changes {
			if verdict := shipped.Decide(agent.Profile, policy.Call{Tool: "Bash", Input: command, Workspace: "/w"}); verdict.Decision != policy.Ask {
				t.Errorf("%s (%s) / %q: got %s (%s), want ask", agent.Name, agent.Profile, command, verdict.Decision, verdict.Reason())
			}
		}
	}
}
