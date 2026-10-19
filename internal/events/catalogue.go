package events

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Spec is what the catalogue knows about one event type: the data it must
// carry, whether it needs a correlation, and who may write it. Rules that
// depend on what happened before (order) live in package rules.
type Spec struct {
	Fields      []string
	Correlation bool
	Writers     []ActorKind
}

// A Registration is what a feature adds to the catalogue: its own streams,
// each named Prefix and an ID and holding only Types, and the TaskTypes it
// writes in task streams, in a namespace of its own ("git.pr_opened").
type Registration struct {
	Prefix    string
	Types     map[Type]Spec
	TaskTypes map[Type]Spec
}

// A Catalogue is every event type a log may hold: the core's and each
// registered feature's. Nothing changes it once it is made, so it stays
// the same for as long as a log is open.
type Catalogue struct {
	specs    map[Type]Spec
	prefixOf map[Type]string // a feature's stream types: the prefix of the streams they go in
	prefixes []string
}

// Core is the catalogue of the core's types alone, with no feature registered.
func Core() *Catalogue {
	c, err := NewCatalogue()
	if err != nil {
		panic(err) // the core's own types can't collide
	}
	return c
}

// NewCatalogue is the core's types plus each registration's. It refuses a
// prefix or type already taken, a prefix that would make its streams look
// like tasks, and a task type in a namespace the core uses in task streams.
func NewCatalogue(extra ...Registration) (*Catalogue, error) {
	c := &Catalogue{specs: maps.Clone(coreTypes), prefixOf: map[Type]string{}}
	taskNamespaces := coreTaskNamespaces()
	for _, r := range extra {
		if err := c.addPrefix(r); err != nil {
			return nil, err
		}
		for _, kind := range slices.Sorted(maps.Keys(r.Types)) {
			if err := c.add(kind, r.Types[kind]); err != nil {
				return nil, err
			}
			c.prefixOf[kind] = r.Prefix
		}
		for _, kind := range slices.Sorted(maps.Keys(r.TaskTypes)) {
			if taskNamespaces[namespace(kind)] {
				return nil, fmt.Errorf("task type %s is in the %s. namespace, which the core uses in task streams", kind, namespace(kind))
			}
			if err := c.add(kind, r.TaskTypes[kind]); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

func (c *Catalogue) addPrefix(r Registration) error {
	switch {
	case r.Prefix == "" && len(r.Types) > 0:
		return fmt.Errorf("stream types %v need a stream prefix", slices.Sorted(maps.Keys(r.Types)))
	case r.Prefix == "":
		return nil
	case strings.HasPrefix(r.Prefix, "task-") || strings.HasPrefix("task-", r.Prefix):
		return fmt.Errorf("stream prefix %q overlaps task-, which names tasks", r.Prefix)
	}
	for _, stream := range coreStreams {
		if strings.HasPrefix(stream, r.Prefix) {
			return fmt.Errorf("stream prefix %q is taken: the core's %q stream starts with it", r.Prefix, stream)
		}
	}
	for _, taken := range c.prefixes {
		if strings.HasPrefix(taken, r.Prefix) || strings.HasPrefix(r.Prefix, taken) {
			return fmt.Errorf("stream prefix %q is taken: it overlaps %q", r.Prefix, taken)
		}
	}
	c.prefixes = append(c.prefixes, r.Prefix)
	return nil
}

func (c *Catalogue) add(kind Type, spec Spec) error {
	if _, taken := c.specs[kind]; taken {
		return fmt.Errorf("event type %s is taken", kind)
	}
	c.specs[kind] = spec
	return nil
}

// coreTaskNamespaces are the namespaces of the core's types that go in task
// streams: "task", "turn", "action" and the rest.
func coreTaskNamespaces() map[string]bool {
	namespaces := map[string]bool{}
	for kind := range coreTypes {
		if _, fixed := streamOf[kind]; !fixed {
			namespaces[namespace(kind)] = true
		}
	}
	return namespaces
}

// namespace is the part of a type before its first dot: "task" for task.created.
func namespace(kind Type) string {
	before, _, _ := strings.Cut(string(kind), ".")
	return before
}

// coreTypes are the core's own event types, in every catalogue.
var coreTypes = map[Type]Spec{
	AgentCreated: {Fields: []string{"name", "prompt", "engine"}, Writers: []ActorKind{Human, System}},
	AgentChanged: {Fields: []string{"name", "prompt", "engine"}, Writers: []ActorKind{Human, System}},
	AgentRemoved: {Fields: []string{"name"}, Writers: []ActorKind{Human}},
	AgentLearned: {Fields: []string{"name"}, Writers: []ActorKind{Human, Agent}}, // "knowledge" may be empty
	AgentRenamed: {Fields: []string{"name", "to"}, Writers: []ActorKind{Human}},

	TaskCreated:        {Fields: []string{"title", "repo", "coordinator"}, Writers: []ActorKind{Human, Agent, System}}, // an agent's: with "parent", for another team, or "epic", a story; yours may carry the "idea" it is; Agentry's: "about" the epic you closed, at "close", or the "idea" the Visionaries agreed on by your rule
	TaskRenamed:        {Fields: []string{"title"}, Writers: []ActorKind{Agent, Human}},
	TaskDirected:       {Fields: []string{"text"}, Writers: []ActorKind{Human}}, // with a board move's "column", "sentBack"
	TaskLabeled:        {Writers: []ActorKind{Human}},                           // "labels" may be empty: that removes them all
	TaskBudgeted:       {Fields: []string{"budget"}, Writers: []ActorKind{Human}},
	TaskTurnsLimited:   {Fields: []string{"maxTurns"}, Writers: []ActorKind{Human}},
	TaskRanked:         {Fields: []string{"rank"}, Writers: []ActorKind{Human}},
	TaskHeld:           {Writers: []ActorKind{Human, System}},                          // "place" "" lets it go, with a board move's "column", "sentBack"; Agentry's: starting a task it made from an idea
	TaskWrappedUp:      {Writers: []ActorKind{Agent, System}},                          // "summary" from the coordinator; Agentry's "reason" when it had none
	TaskReopened:       {Fields: []string{"text"}, Writers: []ActorKind{Human}},        // with a board move's "column", "sentBack"
	RepoChanged:        {Fields: []string{"repo"}, Writers: []ActorKind{Human, Agent}}, // an agent: the coordinator, to a repo you named
	AgentRenamedInTask: {Fields: []string{"name", "to"}, Writers: []ActorKind{Human}},
	TaskTransferred:    {Fields: []string{"coordinator", "reason"}, Writers: []ActorKind{Agent}},
	TaskRequested:      {Fields: []string{"child", "team", "request", "coordinator"}, Writers: []ActorKind{Agent}},
	RequestAnswered:    {Fields: []string{"child", "summary", "coordinator"}, Writers: []ActorKind{System}},
	TaskSplit:          {Fields: []string{"stories"}, Writers: []ActorKind{Agent}},
	StoryDone:          {Fields: []string{"story", "summary", "coordinator"}, Writers: []ActorKind{System}},
	EpicWaits:          {Writers: []ActorKind{Agent}}, // "note" may say what for
	EpicJoined:         {Fields: []string{"epic"}, Writers: []ActorKind{Human}},
	EpicLeft:           {Fields: []string{"epic"}, Writers: []ActorKind{Human}},
	StoryAdded:         {Fields: []string{"story", "title"}, Writers: []ActorKind{Human}}, // "done" and "summary" when it is done already
	StoryRemoved:       {Fields: []string{"story"}, Writers: []ActorKind{Human}},
	FollowUpsSkipped:   {Fields: []string{"reason", "close"}, Writers: []ActorKind{System}},
	FileAdded:          {Fields: []string{"name", "hash", "size"}, Writers: []ActorKind{Human, System}}, // "by": the agent, or you; "turn"
	FileRemoved:        {Fields: []string{"name"}, Writers: []ActorKind{Human, System}},
	FilesCollected:     {Fields: []string{"turn"}, Writers: []ActorKind{System}},
	FilesDelivered:     {Fields: []string{"worker", "version"}, Writers: []ActorKind{System}},
	FileFetching:       {Fields: []string{"machine", "path"}, Writers: []ActorKind{Agent}},       // "as": the name to keep it by
	FilesFetched:       {Fields: []string{"files", "from"}, Writers: []ActorKind{Human, System}}, // yours when you approved it: "answers"
	WorktreeReady:      {Fields: []string{"path", "branch", "port"}, Writers: []ActorKind{Worker}},
	WorktreeMoved:      {Fields: []string{"worker", "path", "port"}, Writers: []ActorKind{Worker}},

	TaskAssigned:       {Fields: []string{"agent", "instructions"}, Writers: []ActorKind{Agent}}, // "checks": the agent checks work; "ships": it ships work
	TurnStarted:        {Fields: []string{"agent", "hash"}, Correlation: true, Writers: []ActorKind{Worker}},
	PromptAcknowledged: {Fields: []string{"hash"}, Correlation: true, Writers: []ActorKind{Worker}},
	SessionStarted:     {Fields: []string{"engineSession"}, Correlation: true, Writers: []ActorKind{Worker}},

	ActionRequested: {Fields: []string{"tool", "command"}, Correlation: true, Writers: []ActorKind{Agent}},
	ActionApproved:  {Fields: []string{"reason"}, Correlation: true, Writers: []ActorKind{Policy, Human}},
	ActionAsked:     {Fields: []string{"reason"}, Correlation: true, Writers: []ActorKind{Policy}},
	ActionDenied:    {Fields: []string{"reason"}, Correlation: true, Writers: []ActorKind{Policy, Human}},
	ActionResult:    {Correlation: true, Writers: []ActorKind{Worker}},

	PlanUpdated:  {Correlation: true, Writers: []ActorKind{Worker}}, // "plan" may be empty: the agent cleared it
	AgentWaiting: {Fields: []string{"question"}, Correlation: true, Writers: []ActorKind{Worker, System}},
	TurnEnded:    {Fields: []string{"agent", "report"}, Correlation: true, Writers: []ActorKind{Worker}},
	IssueRaised:  {Fields: []string{"reason"}, Writers: []ActorKind{Worker, System}},
	TurnStopped:  {Correlation: true, Writers: []ActorKind{Human}},

	TaskNeedsYou: {Fields: []string{"question"}, Writers: []ActorKind{Agent, System}}, // Agentry's: "limit" says which stopped the task
	TaskAnswered: {Fields: []string{"answer"}, Writers: []ActorKind{Human}},           // with a board move's "column", "sentBack"
	TaskDone:     {Fields: []string{"summary"}, Writers: []ActorKind{Agent, Human}},   // yours: an epic you approved closing, or a card you moved to Done ("moved")

	WorkerInvited:      {Fields: []string{"code", "expires"}, Writers: []ActorKind{Human}},
	ConnectionInvited:  {Fields: []string{"code", "expires"}, Writers: []ActorKind{Human}},
	ConnectionMade:     {Fields: []string{"id", "code", "credential"}, Writers: []ActorKind{System}},
	ConnectionRevoked:  {Fields: []string{"id"}, Writers: []ActorKind{Human}},
	WorkerJoined:       {Fields: []string{"name", "code", "credential"}, Writers: []ActorKind{System}},
	WorkerConnected:    {Fields: []string{"name"}, Writers: []ActorKind{System}},
	WorkerDisconnected: {Fields: []string{"name"}, Writers: []ActorKind{System}},
	WorkerRemoved:      {Fields: []string{"name"}, Writers: []ActorKind{Human, System}}, // System: the worker left by itself

	PolicyLoaded:  {Fields: []string{"hash", "path"}, Writers: []ActorKind{System, Human}}, // "text", "change", "diff", "backup", "onDisk"; Human: a change made on the Policy page
	PolicyRefused: {Fields: []string{"reason", "path"}, Writers: []ActorKind{System}},

	SettingsChanged: {Writers: []ActorKind{Human}}, // e.g. "visionariesOnEpicClose", "autoAcceptIdeas", "autoStartIdeas", "autoAcceptRule"

	ProposalRejected: {Writers: nil},
}

// streamOf is the stream a type must be in; types not listed go in a task's stream.
var streamOf = map[Type]string{
	AgentCreated: AgentsStream, AgentChanged: AgentsStream, AgentRemoved: AgentsStream, AgentLearned: AgentsStream, AgentRenamed: AgentsStream,
	PolicyLoaded: PolicyStream, PolicyRefused: PolicyStream,
	WorkerInvited: WorkersStream, WorkerJoined: WorkersStream, WorkerConnected: WorkersStream,
	WorkerDisconnected: WorkersStream, WorkerRemoved: WorkersStream,
	ConnectionInvited: ConnectionsStream, ConnectionMade: ConnectionsStream, ConnectionRevoked: ConnectionsStream,
	SettingsChanged: SettingsStream,
}

// coreStreams are the streams that aren't tasks, each holding the types streamOf puts there.
var coreStreams = []string{AgentsStream, PolicyStream, WorkersStream, ConnectionsStream, SettingsStream}

// IsTaskStream is false for the core's streams that aren't tasks, and for
// every stream a feature registered a prefix for.
func (c *Catalogue) IsTaskStream(stream string) bool {
	if slices.Contains(coreStreams, stream) {
		return false
	}
	return !slices.ContainsFunc(c.prefixes, func(prefix string) bool { return strings.HasPrefix(stream, prefix) })
}

// CheckShape is check 1: the type is known and the proposal carries what it must.
func (c *Catalogue) CheckShape(p Proposal) *Rejection {
	kind, known := c.specs[p.Type]
	if !known {
		return &Rejection{"shape", fmt.Sprintf("unknown event type %q", p.Type)}
	}
	if p.Stream == "" || p.Actor.Kind == "" || p.Actor.ID == "" {
		return &Rejection{"shape", "stream and actor are required"}
	}
	for _, field := range kind.Fields {
		if _, present := p.Data[field]; !present {
			return &Rejection{"shape", fmt.Sprintf("%s needs data.%s", p.Type, field)}
		}
	}
	if kind.Correlation && p.Correlation == "" {
		return &Rejection{"shape", fmt.Sprintf("%s needs a correlation", p.Type)}
	}
	if !c.belongsIn(p.Type, p.Stream) {
		return &Rejection{"shape", fmt.Sprintf("%s does not belong in stream %q", p.Type, p.Stream)}
	}
	return nil
}

// belongsIn says whether a type may be written to a stream. A rejection
// goes in the stream of what it refused.
func (c *Catalogue) belongsIn(kind Type, stream string) bool {
	if want, fixed := streamOf[kind]; fixed {
		return stream == want
	}
	if prefix, owned := c.prefixOf[kind]; owned {
		return strings.HasPrefix(stream, prefix)
	}
	if kind == ProposalRejected {
		return true
	}
	return c.IsTaskStream(stream)
}

// CheckWriter is check 2: this kind of actor may write this type at all.
// Which agent may write a type (only the coordinator assigns) is checked in package rules.
func (c *Catalogue) CheckWriter(p Proposal) *Rejection {
	if !slices.Contains(c.specs[p.Type].Writers, p.Actor.Kind) {
		return &Rejection{"who", fmt.Sprintf("%s may not write %s", p.Actor, p.Type)}
	}
	return nil
}
