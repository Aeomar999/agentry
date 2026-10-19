package events

import (
	"fmt"
	"slices"
	"strings"
)

// spec is what the catalogue knows about one event type: the data it must
// carry, whether it needs a correlation, and who may write it. Rules that
// depend on what happened before (order) live in package rules.
type spec struct {
	fields      []string
	correlation bool
	writers     []ActorKind
}

var catalogue = map[Type]spec{
	AgentCreated: {fields: []string{"name", "prompt", "engine"}, writers: []ActorKind{Human, System}},
	AgentChanged: {fields: []string{"name", "prompt", "engine"}, writers: []ActorKind{Human, System}},
	AgentRemoved: {fields: []string{"name"}, writers: []ActorKind{Human}},
	AgentLearned: {fields: []string{"name"}, writers: []ActorKind{Human, Agent}}, // "knowledge" may be empty
	AgentRenamed: {fields: []string{"name", "to"}, writers: []ActorKind{Human}},

	TaskCreated:        {fields: []string{"title", "repo", "coordinator"}, writers: []ActorKind{Human, Agent, System}}, // an agent's: with "parent", for another team, or "epic", a story; yours may carry the "idea" it is; Agentry's: "about" the epic you closed, at "close", or the "idea" every Visionary agreed on
	TaskRenamed:        {fields: []string{"title"}, writers: []ActorKind{Agent, Human}},
	TaskDirected:       {fields: []string{"text"}, writers: []ActorKind{Human}}, // with a board move's "column", "sentBack"
	TaskLabeled:        {writers: []ActorKind{Human}},                           // "labels" may be empty: that removes them all
	TaskBudgeted:       {fields: []string{"budget"}, writers: []ActorKind{Human}},
	TaskTurnsLimited:   {fields: []string{"maxTurns"}, writers: []ActorKind{Human}},
	TaskRanked:         {fields: []string{"rank"}, writers: []ActorKind{Human}},
	TaskHeld:           {writers: []ActorKind{Human, System}},                          // "place" "" lets it go, with a board move's "column", "sentBack"; Agentry's: starting a task it made from an idea
	TaskWrappedUp:      {writers: []ActorKind{Agent, System}},                          // "summary" from the coordinator; Agentry's "reason" when it had none
	TaskReopened:       {fields: []string{"text"}, writers: []ActorKind{Human}},        // with a board move's "column", "sentBack"
	RepoChanged:        {fields: []string{"repo"}, writers: []ActorKind{Human, Agent}}, // an agent: the coordinator, to a repo you named
	AgentRenamedInTask: {fields: []string{"name", "to"}, writers: []ActorKind{Human}},
	TaskTransferred:    {fields: []string{"coordinator", "reason"}, writers: []ActorKind{Agent}},
	TaskRequested:      {fields: []string{"child", "team", "request", "coordinator"}, writers: []ActorKind{Agent}},
	RequestAnswered:    {fields: []string{"child", "summary", "coordinator"}, writers: []ActorKind{System}},
	TaskSplit:          {fields: []string{"stories"}, writers: []ActorKind{Agent}},
	StoryDone:          {fields: []string{"story", "summary", "coordinator"}, writers: []ActorKind{System}},
	EpicWaits:          {writers: []ActorKind{Agent}}, // "note" may say what for
	EpicJoined:         {fields: []string{"epic"}, writers: []ActorKind{Human}},
	EpicLeft:           {fields: []string{"epic"}, writers: []ActorKind{Human}},
	StoryAdded:         {fields: []string{"story", "title"}, writers: []ActorKind{Human}}, // "done" and "summary" when it is done already
	StoryRemoved:       {fields: []string{"story"}, writers: []ActorKind{Human}},
	FollowUpsSkipped:   {fields: []string{"reason", "close"}, writers: []ActorKind{System}},
	FileAdded:          {fields: []string{"name", "hash", "size"}, writers: []ActorKind{Human, System}}, // "by": the agent, or you; "turn"
	FileRemoved:        {fields: []string{"name"}, writers: []ActorKind{Human, System}},
	FilesCollected:     {fields: []string{"turn"}, writers: []ActorKind{System}},
	FilesDelivered:     {fields: []string{"worker", "version"}, writers: []ActorKind{System}},
	FileFetching:       {fields: []string{"machine", "path"}, writers: []ActorKind{Agent}},       // "as": the name to keep it by
	FilesFetched:       {fields: []string{"files", "from"}, writers: []ActorKind{Human, System}}, // yours when you approved it: "answers"
	WorktreeReady:      {fields: []string{"path", "branch", "port"}, writers: []ActorKind{Worker}},
	WorktreeMoved:      {fields: []string{"worker", "path", "port"}, writers: []ActorKind{Worker}},

	TaskAssigned:       {fields: []string{"agent", "instructions"}, writers: []ActorKind{Agent}}, // "checks": the agent checks work; "ships": it ships work
	TurnStarted:        {fields: []string{"agent", "hash"}, correlation: true, writers: []ActorKind{Worker}},
	PromptAcknowledged: {fields: []string{"hash"}, correlation: true, writers: []ActorKind{Worker}},
	SessionStarted:     {fields: []string{"engineSession"}, correlation: true, writers: []ActorKind{Worker}},

	ActionRequested: {fields: []string{"tool", "command"}, correlation: true, writers: []ActorKind{Agent}},
	ActionApproved:  {fields: []string{"reason"}, correlation: true, writers: []ActorKind{Policy, Human}},
	ActionAsked:     {fields: []string{"reason"}, correlation: true, writers: []ActorKind{Policy}},
	ActionDenied:    {fields: []string{"reason"}, correlation: true, writers: []ActorKind{Policy, Human}},
	ActionResult:    {correlation: true, writers: []ActorKind{Worker}},

	PlanUpdated:  {correlation: true, writers: []ActorKind{Worker}}, // "plan" may be empty: the agent cleared it
	AgentWaiting: {fields: []string{"question"}, correlation: true, writers: []ActorKind{Worker, System}},
	TurnEnded:    {fields: []string{"agent", "report"}, correlation: true, writers: []ActorKind{Worker}},
	TurnLimited:  {fields: []string{"agent", "error", "details", "resetsAt"}, correlation: true, writers: []ActorKind{Worker}}, // "resetsAt" "" when the engine didn't say
	IssueRaised:  {fields: []string{"reason"}, writers: []ActorKind{Worker, System}},
	TurnStopped:  {correlation: true, writers: []ActorKind{Human}},

	TaskNeedsYou: {fields: []string{"question"}, writers: []ActorKind{Agent, System}}, // Agentry's: "limit" says which stopped the task
	TaskAnswered: {fields: []string{"answer"}, writers: []ActorKind{Human}},           // with a board move's "column", "sentBack"
	TaskDone:     {fields: []string{"summary"}, writers: []ActorKind{Agent, Human}},   // yours: an epic you approved closing, or a card you moved to Done ("moved")

	WorkerInvited:      {fields: []string{"code", "expires"}, writers: []ActorKind{Human}},
	ConnectionInvited:  {fields: []string{"code", "expires"}, writers: []ActorKind{Human}},
	ConnectionMade:     {fields: []string{"id", "code", "credential"}, writers: []ActorKind{System}},
	ConnectionRevoked:  {fields: []string{"id"}, writers: []ActorKind{Human}},
	WorkerJoined:       {fields: []string{"name", "code", "credential"}, writers: []ActorKind{System}},
	WorkerConnected:    {fields: []string{"name"}, writers: []ActorKind{System}},
	WorkerDisconnected: {fields: []string{"name"}, writers: []ActorKind{System}},
	WorkerRemoved:      {fields: []string{"name"}, writers: []ActorKind{Human, System}}, // System: the worker left by itself

	PolicyLoaded:  {fields: []string{"hash", "path"}, writers: []ActorKind{System, Human}}, // "text", "change", "diff", "backup"; Human: a change made on the Policy page
	PolicyRefused: {fields: []string{"reason", "path"}, writers: []ActorKind{System}},

	IdeaSuggested: {fields: []string{"title", "problem", "proposal", "size", "repo", "task"}, writers: []ActorKind{Agent, System}},
	IdeaAccepted:  {fields: []string{"task"}, writers: []ActorKind{Human, System}}, // "note" may be empty; Agentry's: an idea every Visionary agreed on
	IdeaDismissed: {fields: []string{"note"}, writers: []ActorKind{Human}},

	SettingsChanged: {writers: []ActorKind{Human}}, // e.g. "visionariesOnEpicClose", "autoAcceptIdeas", "autoStartIdeas"

	ProposalRejected: {writers: nil},
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

// ideaTypes go in an idea's stream.
var ideaTypes = []Type{IdeaSuggested, IdeaAccepted, IdeaDismissed}

// IsTaskStream is false for the agents, policy, workers, connections and
// settings streams, and for ideas.
func IsTaskStream(stream string) bool {
	return stream != AgentsStream && stream != PolicyStream && stream != WorkersStream && stream != ConnectionsStream &&
		stream != SettingsStream && !IsIdeaStream(stream)
}

func IsIdeaStream(stream string) bool { return strings.HasPrefix(stream, IdeaPrefix) }

// CheckShape is check 1: the type is known and the proposal carries what it must.
func CheckShape(p Proposal) *Rejection {
	kind, known := catalogue[p.Type]
	if !known {
		return &Rejection{"shape", fmt.Sprintf("unknown event type %q", p.Type)}
	}
	if p.Stream == "" || p.Actor.Kind == "" || p.Actor.ID == "" {
		return &Rejection{"shape", "stream and actor are required"}
	}
	for _, field := range kind.fields {
		if _, present := p.Data[field]; !present {
			return &Rejection{"shape", fmt.Sprintf("%s needs data.%s", p.Type, field)}
		}
	}
	if kind.correlation && p.Correlation == "" {
		return &Rejection{"shape", fmt.Sprintf("%s needs a correlation", p.Type)}
	}
	if !belongsIn(p.Type, p.Stream) {
		return &Rejection{"shape", fmt.Sprintf("%s does not belong in stream %q", p.Type, p.Stream)}
	}
	return nil
}

// belongsIn says whether a type may be written to a stream. A rejection
// goes in the stream of what it refused.
func belongsIn(kind Type, stream string) bool {
	if want, fixed := streamOf[kind]; fixed {
		return stream == want
	}
	switch {
	case kind == ProposalRejected:
		return true
	case slices.Contains(ideaTypes, kind):
		return IsIdeaStream(stream)
	}
	return IsTaskStream(stream)
}

// CheckWriter is check 2: this kind of actor may write this type at all.
// Which agent may write a type (only the coordinator assigns) is checked in package rules.
func CheckWriter(p Proposal) *Rejection {
	if !slices.Contains(catalogue[p.Type].writers, p.Actor.Kind) {
		return &Rejection{"who", fmt.Sprintf("%s may not write %s", p.Actor, p.Type)}
	}
	return nil
}
