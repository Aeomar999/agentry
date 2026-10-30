package coordinator

import "testing"

func TestVerdictOf(t *testing.T) {
	for report, want := range map[string]Verdict{
		"Looks good.\n\nVerdict: approved":                    Approved,
		"Findings…\n**Verdict: Changes needed** (must fix 2)": ChangesNeeded,
		"## Verdict: not approved":                            ChangesNeeded,
		"verdict - approved, with two nits":                   Approved,
		"I approved of the naming.":                           NoVerdict,
		"Verdict: approved\n\nMore text after.\n\nAnd more.":  Approved,
		"": NoVerdict,
	} {
		if got := verdictOf(report); got != want {
			t.Errorf("verdictOf(%q) = %q, want %q", report, got, want)
		}
	}
}
