package coordinator

import "testing"

func TestParseDecision(t *testing.T) {
	cases := []struct {
		name    string
		reply   string
		want    Decision
		wantErr bool
	}{
		{"plain JSON last",
			"The engineer should start.\n\n" + `{"action": "assign", "agent": "engineer", "instructions": "Remove the TODOs in src/."}`,
			Decision{Action: Assign, Agent: "engineer", Instructions: "Remove the TODOs in src/."}, false},
		{"in a fenced block",
			"Done.\n```json\n" + `{"action": "done", "summary": "TODOs removed and deployed"}` + "\n```\n",
			Decision{Action: Finish, Summary: "TODOs removed and deployed"}, false},
		{"braces earlier in the reply",
			"The handler `func() {}` looks fine. {not json}\n" + `{"action": "ask", "question": "Deploy on a Friday?"}`,
			Decision{Action: Ask, Question: "Deploy on a Friday?"}, false},
		{"instructions containing braces",
			`{"action": "assign", "agent": "tester", "instructions": "Check that {\"a\": 1} round-trips"}`,
			Decision{Action: Assign, Agent: "tester", Instructions: `Check that {"a": 1} round-trips`}, false},
		{"no decision", "I think the engineer should go next.", Decision{}, true},
		{"text after the JSON", `{"action": "done", "summary": "x"} and then more words`, Decision{}, true},
		{"assign without an agent", `{"action": "assign", "instructions": "go"}`, Decision{}, true},
		{"unknown action", `{"action": "deploy"}`, Decision{}, true},
	}
	for _, c := range cases {
		got, err := ParseDecision(c.reply)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: error %v, want error %v", c.name, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
