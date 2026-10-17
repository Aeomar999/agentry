package events

import (
	"fmt"
	"slices"
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

	TaskCreated:        {fields: []string{"title", "repo", "coordinator"}, writers: []ActorKind{Human, Agent}}, // an agent's: with "parent", for another team, or "epic", a story
	TaskRenamed:        {fields: []string{"title"}, writers: []ActorKind{Agent, Human}},
	TaskDirected:       {fields: []string{"text"}, writers: []ActorKind{Human}},
	TaskLabeled:        {writers: []ActorKind{Human}}, // "labels" may be empty: that removes them all
	TaskBudgeted:       {fields: []string{"budget"}, writers: []ActorKind{Human}},
	TaskRanked:         {fields: []string{"rank"}, writers: []ActorKind{Human}},
	TaskHeld:           {writers: []ActorKind{Human}}, // "place" "" lets it go
	TaskReopened:       {fields: []string{"text"}, writers: []ActorKind{Human}},
	RepoChanged:        {fields: []string{"repo"}, writers: []ActorKind{Human}},
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
	FileAdded:          {fields: []string{"name", "hash", "size"}, writers: []ActorKind{Human, System}}, // "by": the agent, or you; "turn"
	FileRemoved:        {fields: []string{"name"}, writers: []ActorKind{Human, System}},
	FilesCollected:     {fields: []string{"turn"}, writers: []ActorKind{System}},
	FilesDelivered:     {fields: []string{"worker", "version"}, writers: []ActorKind{System}},
	FileFetching:       {fields: []string{"machine", "path"}, writers: []ActorKind{Agent}},       // "as": the name to keep it by
	FilesFetched:       {fields: []string{"files", "from"}, writers: []ActorKind{Human, System}}, // yours when you approved it: "answers"
	WorktreeReady:      {fields: []string{"path", "branch", "port"}, writers: []ActorKind{Worker}},
	WorktreeMoved:      {fields: []string{"worker", "path", "port"}, writers: []ActorKind{Worker}},

	TaskAssigned:       {fields: []string{"agent", "instructions"}, writers: []ActorKind{Agent}},
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
	IssueRaised:  {fields: []string{"reason"}, writers: []ActorKind{Worker, System}},
	TurnStopped:  {correlation: true, writers: []ActorKind{Human}},

	TaskNeedsYou: {fields: []string{"question"}, writers: []ActorKind{Agent, System}},
	TaskAnswered: {fields: []string{"answer"}, writers: []ActorKind{Human}},
	TaskDone:     {fields: []string{"summary"}, writers: []ActorKind{Agent, Human}}, // yours: an epic you approved closing

	WorkerInvited:      {fields: []string{"code", "expires"}, writers: []ActorKind{Human}},
	ConnectionInvited:  {fields: []string{"code", "expires"}, writers: []ActorKind{Human}},
	ConnectionMade:     {fields: []string{"id", "code", "credential"}, writers: []ActorKind{System}},
	ConnectionRevoked:  {fields: []string{"id"}, writers: []ActorKind{Human}},
	WorkerJoined:       {fields: []string{"name", "code", "credential"}, writers: []ActorKind{System}},
	WorkerConnected:    {fields: []string{"name"}, writers: []ActorKind{System}},
	WorkerDisconnected: {fields: []string{"name"}, writers: []ActorKind{System}},
	WorkerRemoved:      {fields: []string{"name"}, writers: []ActorKind{Human, System}}, // System: the worker left by itself

	PolicyLoaded:  {fields: []string{"hash", "path"}, writers: []ActorKind{System}},
	PolicyRefused: {fields: []string{"reason", "path"}, writers: []ActorKind{System}},

	ProposalRejected: {writers: nil},
}

// streamOf is the stream a type must be in; types not listed go in a task's stream.
var streamOf = map[Type]string{
	AgentCreated: AgentsStream, AgentChanged: AgentsStream, AgentRemoved: AgentsStream, AgentLearned: AgentsStream, AgentRenamed: AgentsStream,
	PolicyLoaded: PolicyStream, PolicyRefused: PolicyStream,
	WorkerInvited: WorkersStream, WorkerJoined: WorkersStream, WorkerConnected: WorkersStream,
	WorkerDisconnected: WorkersStream, WorkerRemoved: WorkersStream,
	ConnectionInvited: ConnectionsStream, ConnectionMade: ConnectionsStream, ConnectionRevoked: ConnectionsStream,
}

// IsTaskStream is false for the agents, policy, workers and connections streams.
func IsTaskStream(stream string) bool {
	return stream != AgentsStream && stream != PolicyStream && stream != WorkersStream && stream != ConnectionsStream
}

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
	if want, fixed := streamOf[p.Type]; (fixed && p.Stream != want) || (!fixed && p.Type != ProposalRejected && !IsTaskStream(p.Stream)) {
		return &Rejection{"shape", fmt.Sprintf("%s does not belong in stream %q", p.Type, p.Stream)}
	}
	return nil
}

// CheckWriter is check 2: this kind of actor may write this type at all.
// Which agent may write a type (only the coordinator assigns) is checked in package rules.
func CheckWriter(p Proposal) *Rejection {
	if !slices.Contains(catalogue[p.Type].writers, p.Actor.Kind) {
		return &Rejection{"who", fmt.Sprintf("%s may not write %s", p.Actor, p.Type)}
	}
	return nil
}
