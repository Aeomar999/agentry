package hooks

import (
	"strings"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Redactions are the secrets you redacted from the agents' knowledge while
// turns were running. Each such turn's knowledge file was written as it
// started, secret and all, and its end would write the secret back as what
// the agent learned; so its end redacts them again first. A secret is
// forgotten once the last turn that was running when you redacted it ends.
//
// They are only ever held here, in memory, and gone when the server
// restarts: a list of redacted secrets, or of their hashes, kept anywhere
// would be a list of them to steal (docs/design/log-growth.md section 5).
type Redactions struct {
	state *current.State // whose open turns are the ones running

	mu   sync.Mutex
	held map[string]map[Turn]bool // each secret, and the turns that may still write it back
}

// A Turn is one task's turn, as its hook calls name it.
type Turn struct{ Task, ID string }

func newRedactions(state *current.State) *Redactions {
	return &Redactions{state: state, held: map[string]map[Turn]bool{}}
}

// Hold keeps secret out of what the turns running now learn, from before
// the redaction commits. settle says whether it did: if so, the turns that
// started meanwhile, from the knowledge as it was, are held too; if not,
// the turns this hold added are let go. A nil Redactions holds nothing.
func (r *Redactions) Hold(secret string) (settle func(committed bool)) {
	if r == nil {
		return func(bool) {}
	}
	added := r.add(secret)
	return func(committed bool) {
		if committed {
			r.add(secret)
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		for turn := range added {
			delete(r.held[secret], turn)
		}
		if len(r.held[secret]) == 0 {
			delete(r.held, secret)
		}
	}
}

// add holds secret for every turn running now, and is the turns that
// weren't holding it already.
func (r *Redactions) add(secret string) map[Turn]bool {
	running := r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	turns := r.held[secret]
	if turns == nil {
		turns = map[Turn]bool{}
	}
	added := map[Turn]bool{}
	for turn := range running {
		if !turns[turn] {
			turns[turn], added[turn] = true, true
		}
	}
	if len(turns) > 0 {
		r.held[secret] = turns
	}
	return added
}

// Holds says whether secret is held for a turn that is still to end.
func (r *Redactions) Holds(secret string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.held[secret]) > 0
}

// Ended redacts each secret held for turn from learned, what it left in its
// knowledge file, and forgets the turn, and any other held turn that has
// ended since. It reports false when a secret would still show in the text
// next to store.Redacted, as the store refuses to leave it: then nothing the
// turn learned may be kept.
func (r *Redactions) Ended(turn Turn, learned string) (string, bool) {
	running := r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	clean := true
	for secret, turns := range r.held {
		if turns[turn] {
			learned = strings.ReplaceAll(learned, secret, store.Redacted)
			clean = clean && !strings.Contains(learned, secret)
		}
		for held := range turns {
			if held == turn || !running[held] {
				delete(turns, held)
			}
		}
		if len(turns) == 0 {
			delete(r.held, secret)
		}
	}
	return learned, clean
}

// running is every turn open now.
func (r *Redactions) running() map[Turn]bool {
	open := map[Turn]bool{}
	for _, task := range r.state.Tasks() {
		if turn := task.OpenTurn(); turn != nil {
			open[Turn{task.ID, turn.ID}] = true
		}
	}
	return open
}
