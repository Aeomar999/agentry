package web

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/settings"
)

// settingsView is your settings as the page shows and changes them.
type settingsView struct {
	VisionariesOnEpicClose bool `json:"visionariesOnEpicClose"` // a Visionaries session after you close an epic
	AutoAcceptIdeas        bool `json:"autoAcceptIdeas"`        // Agentry accepts an idea every Visionary backs
	AutoStartIdeas         bool `json:"autoStartIdeas"`         // and starts its task
}

func (s *Server) showSettings(w http.ResponseWriter, r *http.Request) {
	now := s.state.Settings()
	reply(w, settingsView{VisionariesOnEpicClose: now.VisionariesOnEpicClose, AutoAcceptIdeas: now.AutoAcceptIdeas,
		AutoStartIdeas: now.AutoStartIdeas})
}

// changeSettings changes the settings the request names, 400 when it names
// none, and answers with all of them as they are now.
func (s *Server) changeSettings(w http.ResponseWriter, r *http.Request) {
	var request struct{ VisionariesOnEpicClose, AutoAcceptIdeas, AutoStartIdeas *bool }
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
	if len(changes) == 0 {
		http.Error(w, "say which setting to change: visionariesOnEpicClose, autoAcceptIdeas or autoStartIdeas", http.StatusBadRequest)
		return
	}
	if !s.proposeOrRefuse(w, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: changes}) {
		return
	}
	s.showSettings(w, r)
}
