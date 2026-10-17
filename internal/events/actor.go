package events

// ActorKind is who is behind a proposal.
type ActorKind string

const (
	Human  ActorKind = "human"
	Agent  ActorKind = "agent"  // an AI agent; its ID is the agent's name
	Worker ActorKind = "worker" // the machine running the agents' sessions
	Policy ActorKind = "policy" // the allow / ask / deny rules deciding on their own
	System ActorKind = "system" // Agentry itself: the coordinator loop, the watchdog, the validator
)

type Actor struct {
	Kind ActorKind `json:"kind"`
	ID   string    `json:"id"`
}

func (a Actor) String() string { return string(a.Kind) + ":" + a.ID }
