// Package settingsapi is the settings' HTTP API: GET and PUT /api/settings,
// as the Settings page reads and changes them, and redacting a secret from
// a task, the agents' stream or the whole log. Web mounts its Routes as it
// mounts an extension's.
package settingsapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

// State is what the routes read: the settings, and the tasks and roster
// that say how many tasks are running; and, for a redaction, each task,
// whether it was erased and its summary, and the streams an extension owns.
type State interface {
	Settings() settings.Settings
	Tasks() []*tasks.Task
	Roster() agents.Roster
	Task(id string) (*tasks.Task, error)
	TaskIDs() []string
	Erased(id string) (tasks.Erasure, bool)
	OwnedIDs(prefix string) []string
	Summary(id string) (tasks.Summary, bool)
}

// Store is the log's side of a redaction: counting the rows that hold a
// secret, and rewriting them, a live stream's or an archived task's.
type Store interface {
	Preview(stream, secret string) (int, *events.Rejection, error)
	PreviewArchived(stream, secret string, blobs store.Opener) (int, *events.Rejection, error)
	RedactCounted(stream, secret string, rows int, who events.Actor, why string) (events.Event, *events.Rejection, error)
	RedactArchivedCounted(stream, secret string, rows int, who events.Actor, why string, summarise func() any, blobs store.ArchiveBlobs) (events.Event, *events.Rejection, error)
}

// Redactions holds a secret redacted from the agents' knowledge for the
// turns running then, whose ends redact it again (hooks.Redactions).
type Redactions interface {
	Hold(secret string) (settle func(committed bool))
	Holds(secret string) bool
}

// API holds what the settings' routes need: the log to write their changes
// to, and the state to read them back from; and, for a redaction, the log
// to redact, where archived tasks' blobs are kept, the secrets held for
// running turns, the extensions whose streams it reads, and web's own
// steps once it commits.
type API struct {
	Log        extensions.Proposer
	State      State
	Store      Store
	Blobs      sharedfiles.Disk
	Redactions Redactions
	Extensions []extensions.Extension
	// Forget drops a stream from web's cache of opened archives.
	Forget func(stream string)
	// WriteOverOldText gives the database's free space back and writes a
	// new snapshot, as erasing a task does, and is what failed, as warnings.
	WriteOverOldText func() []string
}

// Routes are the settings' endpoints, for the page and Claude Code.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/settings", Auth: extensions.UI, Handler: a.showSettings},
		{Pattern: "PUT /api/settings", Auth: extensions.UI, Handler: a.changeSettings},
		{Pattern: "POST /api/tasks/{task}/redact/preview", Auth: extensions.UI, Handler: a.previewRedaction},
		{Pattern: "POST /api/tasks/{task}/redact", Auth: extensions.UI, Handler: a.redact}, // never the desk's: nothing undoes it
		{Pattern: "POST /api/agents/redact/preview", Auth: extensions.UI, Handler: a.previewAgentsRedaction},
		{Pattern: "POST /api/agents/redact", Auth: extensions.UI, Handler: a.redactAgents}, // never the desk's: nothing undoes it
		{Pattern: "POST /api/redact/preview", Auth: extensions.UI, Handler: a.previewLogRedaction},
		{Pattern: "POST /api/redact", Auth: extensions.UI, Handler: a.redactLog}, // never the desk's: nothing undoes it
	}
}

// settingsView is your settings as the page shows and changes them.
type settingsView struct {
	VisionariesOnEpicClose bool   `json:"visionariesOnEpicClose"` // a Visionaries session after you close an epic
	AutoAcceptIdeas        bool   `json:"autoAcceptIdeas"`        // Agentry accepts an idea the Visionaries agree on
	AutoStartIdeas         bool   `json:"autoStartIdeas"`         // and starts its task
	AutoAcceptRule         string `json:"autoAcceptRule"`         // which ideas they agree on: everyoneBacked or nobodyDoubted
	MaxRunningTasks        int    `json:"maxRunningTasks"`        // the most tasks with a turn at once; 0 is no limit
	RunningTasks           int    `json:"runningTasks"`           // how many have one now: read only, for "Queued: 3 of 3 running"
}

func (a API) showSettings(w http.ResponseWriter, r *http.Request) {
	now := a.State.Settings()
	httpapi.Reply(w, settingsView{VisionariesOnEpicClose: now.VisionariesOnEpicClose, AutoAcceptIdeas: now.AutoAcceptIdeas,
		AutoStartIdeas: now.AutoStartIdeas, AutoAcceptRule: now.AutoAcceptRule, MaxRunningTasks: now.MaxRunningTasks,
		RunningTasks: coordinator.SlotsOf(a.State.Tasks(), a.State.Roster(), now.MaxRunningTasks).Running})
}

// changeSettings changes the settings the request names, 400 when it names
// none, a rule Agentry doesn't know or a limit of tasks out of range, and
// answers with all of them as they are now.
func (a API) changeSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		VisionariesOnEpicClose, AutoAcceptIdeas, AutoStartIdeas *bool
		AutoAcceptRule                                          *string
		MaxRunningTasks                                         *int
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	changes := map[string]any{}
	for name, value := range map[string]*bool{settings.VisionariesOnEpicClose: request.VisionariesOnEpicClose,
		settings.AutoAcceptIdeas: request.AutoAcceptIdeas, settings.AutoStartIdeas: request.AutoStartIdeas} {
		if value != nil {
			changes[name] = *value
		}
	}
	if rule := request.AutoAcceptRule; rule != nil {
		if !slices.Contains(settings.AutoAcceptRules, *rule) {
			http.Error(w, "autoAcceptRule is one of "+strings.Join(settings.AutoAcceptRules, " or ")+", not "+strconv.Quote(*rule), http.StatusBadRequest)
			return
		}
		changes[settings.AutoAcceptRule] = *rule
	}
	if limit := request.MaxRunningTasks; limit != nil {
		if *limit < 0 || *limit > settings.MostRunningTasks {
			http.Error(w, "maxRunningTasks is from 1 to "+strconv.Itoa(settings.MostRunningTasks)+", or 0 for no limit, not "+strconv.Itoa(*limit), http.StatusBadRequest)
			return
		}
		changes[settings.MaxRunningTasks] = *limit
	}
	if len(changes) == 0 {
		http.Error(w, "say which setting to change: visionariesOnEpicClose, autoAcceptIdeas, autoStartIdeas, autoAcceptRule or maxRunningTasks", http.StatusBadRequest)
		return
	}
	if !httpapi.ProposeOrRefuse(w, a.Log, events.Proposal{Stream: events.SettingsStream, Actor: httpapi.You, Type: events.SettingsChanged, Data: changes}) {
		return
	}
	a.showSettings(w, r)
}
