package store

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// An agent's knowledge is a chain of versions: each agent.learned is a full
// copy of it or a patch on the version before, which keeps that version's
// first and last bytes. A secret written into one version is in every
// version after it that keeps it, though no row after the first holds it,
// and redacting a version's text moves the bytes a patch on it keeps. So a
// patch is written again as a full copy of its version's redacted text when
// that text or the one before it holds the secret, and a full copy holding
// it is redacted in place (docs/design/log-growth.md section 5, "Removing a
// secret", item 4). The versions before the secret are left as they are.

// rewrittenIn is the rows of stream that hold secret, rewritten, in order:
// in the agents stream, the knowledge versions that hold it or whose base
// does too (knowledgeRewrites), in place of what a search of their own data
// finds at their positions, unless that search found it in a key, which a
// full copy would drop: the row is refused for it instead.
func rewrittenIn(stream string, rows []row, secret string) ([]rewrittenRow, error) {
	found, err := rewritten(rows, secret)
	if err != nil || stream != events.AgentsStream {
		return found, err
	}
	versions, err := knowledgeRewrites(rows, secret)
	if err != nil {
		return nil, err
	}
	byPosition := map[int64]rewrittenRow{}
	for _, rw := range found {
		byPosition[rw.position] = rw
	}
	for _, version := range versions {
		if !byPosition[version.position].inKey {
			byPosition[version.position] = version
		}
	}
	return slices.SortedFunc(maps.Values(byPosition), func(a, b rewrittenRow) int { return cmp.Compare(a.position, b.position) }), nil
}

// A version is one agent.learned of an agent, and the text it makes.
type version struct {
	row   row
	name  string // the agent's name then
	text  string
	patch bool
}

// knowledgeRewrites is the agent.learned rows to rewrite for secret: of
// each agent, a patch whose text, or the text before it, holds the secret,
// as a full copy of its text redacted, and a full copy whose text holds it,
// redacted in place.
func knowledgeRewrites(rows []row, secret string) ([]rewrittenRow, error) {
	byAgent, err := knowledgeVersions(rows)
	if err != nil {
		return nil, err
	}
	var rewrites []rewrittenRow
	for _, versions := range byAgent {
		baseHeld := false
		for _, v := range versions {
			held := strings.Contains(v.text, secret)
			switch {
			case v.patch && (held || baseHeld):
				rw, err := fullCopy(v, secret)
				if err != nil {
					return nil, err
				}
				rewrites = append(rewrites, rw)
			case !v.patch && held:
				rw, err := redactedData(v.row.Data, secret)
				if err != nil {
					return nil, fmt.Errorf("the data at %d: %w", v.row.Position, err)
				}
				rw.position, rw.kind = v.row.Position, events.AgentLearned
				rewrites = append(rewrites, rw)
			}
			baseHeld = held
		}
	}
	return rewrites, nil
}

// fullCopy is a patch written again as the full copy of its version's
// text, as Agent.Learned writes one, with secret redacted. It names the
// knowledge field it now has, even when its own text doesn't hold the
// secret but its base did.
func fullCopy(v version, secret string) (rewrittenRow, error) {
	data, err := json.Marshal(map[string]any{"knowledge": v.text, "name": v.name})
	if err != nil {
		return rewrittenRow{}, err
	}
	rw, err := redactedData(string(data), secret)
	if err != nil {
		return rewrittenRow{}, err
	}
	if !rw.holds() {
		rw = rewrittenRow{data: string(data), fields: []string{"$.knowledge"}}
	}
	rw.position, rw.kind = v.row.Position, events.AgentLearned
	return rw, nil
}

// knowledgeVersions is every agent's knowledge versions, in order, from the
// agents stream's rows, rejections included, which it skips. An agent is
// followed across renames, by its name at the time, as Roster.Apply does;
// one removed and made again by the same name is another agent. A version
// for no agent is left out, as the roster ignores it.
func knowledgeVersions(rows []row) ([][]version, error) {
	var byAgent [][]version
	var texts []string
	agentNamed := map[string]int{}
	for _, r := range rows {
		var data map[string]any
		switch events.Type(r.Type) {
		case events.AgentCreated, events.AgentRenamed, events.AgentRemoved, events.AgentLearned:
			if err := json.Unmarshal([]byte(r.Data), &data); err != nil {
				return nil, fmt.Errorf("the data at %d: %w", r.Position, err)
			}
		default:
			continue
		}
		e := events.Proposal{Type: events.Type(r.Type), Data: data}
		name := e.Text("name")
		agent, known := agentNamed[name]
		switch e.Type {
		case events.AgentCreated:
			if !known {
				agentNamed[name] = len(byAgent)
				byAgent, texts = append(byAgent, nil), append(texts, "")
			}
		case events.AgentRenamed:
			if known {
				delete(agentNamed, name)
				agentNamed[e.Text("to")] = agent
			}
		case events.AgentRemoved:
			delete(agentNamed, name)
		case events.AgentLearned:
			if !known {
				continue
			}
			text, err := learned(texts[agent], e)
			if err != nil {
				return nil, fmt.Errorf("%s's knowledge at %d: %w", name, r.Position, err)
			}
			texts[agent] = text
			byAgent[agent] = append(byAgent[agent], version{row: r, name: name, text: text, patch: agents.IsPatch(e)})
		}
	}
	return byAgent, nil
}

// learned is the text an agent.learned makes of the text before it.
func learned(before string, e events.Proposal) (string, error) {
	if !agents.IsPatch(e) {
		return e.Text("knowledge"), nil
	}
	patch, err := agents.PatchOf(e)
	if err != nil {
		return "", err
	}
	return patch.Apply(before)
}
