package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/settings"
)

// settingsView is your settings as the page shows and changes them.
type settingsView struct {
	VisionariesOnEpicClose bool   `json:"visionariesOnEpicClose"` // a Visionaries session after you close an epic
	AutoAcceptIdeas        bool   `json:"autoAcceptIdeas"`        // Agentry accepts an idea the Visionaries agree on
	AutoStartIdeas         bool   `json:"autoStartIdeas"`         // and starts its task
	AutoAcceptRule         string `json:"autoAcceptRule"`         // which ideas they agree on: everyoneBacked or nobodyDoubted
}

func (s *Server) showSettings(w http.ResponseWriter, r *http.Request) {
	now := s.state.Settings()
	reply(w, settingsView{VisionariesOnEpicClose: now.VisionariesOnEpicClose, AutoAcceptIdeas: now.AutoAcceptIdeas,
		AutoStartIdeas: now.AutoStartIdeas, AutoAcceptRule: now.AutoAcceptRule})
}

// changeSettings changes the settings the request names, 400 when it names
// none or a rule Agentry doesn't know, and answers with all of them as they
// are now.
func (s *Server) changeSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		VisionariesOnEpicClose, AutoAcceptIdeas, AutoStartIdeas *bool
		AutoAcceptRule                                          *string
	}
	if !decode(w, r, &request) {
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
	if len(changes) == 0 {
		http.Error(w, "say which setting to change: visionariesOnEpicClose, autoAcceptIdeas, autoStartIdeas or autoAcceptRule", http.StatusBadRequest)
		return
	}
	if !s.proposeOrRefuse(w, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: changes}) {
		return
	}
	s.showSettings(w, r)
}
