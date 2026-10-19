package web

import (
	"errors"
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/webpush"
)

// pushView is whether this device can ask for pushes, as the page shows it.
type pushView struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`    // why not, when not
	PublicKey  string `json:"publicKey,omitempty"` // the VAPID public key the browser subscribes with, when available
	Subscribed int    `json:"subscribed"`          // how many browsers are subscribed
}

// pushAvailable says whether push can work here, and why not: it needs the
// file of subscriptions and an https -public-url.
func (s *Server) pushAvailable() (bool, string) {
	if s.config.Push == nil {
		return false, "This server keeps no push subscriptions."
	}
	return webpush.Available(s.config.PublicURL)
}

func (s *Server) showPush(w http.ResponseWriter, r *http.Request) {
	available, reason := s.pushAvailable()
	view := pushView{Available: available, Reason: reason}
	if s.config.Push != nil {
		view.Subscribed = len(s.config.Push.Subscriptions())
		if available {
			view.PublicKey = s.config.Push.PublicKey()
		}
	}
	httpapi.Reply(w, view)
}

// subscribePush keeps the browser's PushSubscription, as its toJSON gives
// it. It is refused while push is off, and 400 when malformed.
func (s *Server) subscribePush(w http.ResponseWriter, r *http.Request) {
	if available, reason := s.pushAvailable(); !available {
		http.Error(w, reason, http.StatusConflict)
		return
	}
	var subscription webpush.Subscription
	if !httpapi.Decode(w, r, &subscription) {
		return
	}
	if err := s.config.Push.Subscribe(subscription, time.Now()); errors.Is(err, webpush.ErrMalformed) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if err != nil {
		httpapi.Failed(w, err)
		return
	}
	s.showPush(w, r)
}

// unsubscribePush stops the pushes to ?endpoint=, even while push is off.
func (s *Server) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	if s.config.Push == nil {
		http.Error(w, "no such subscription", http.StatusNotFound)
		return
	}
	removed, err := s.config.Push.Unsubscribe(r.URL.Query().Get("endpoint"))
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if !removed {
		http.Error(w, "no such subscription", http.StatusNotFound)
		return
	}
	s.showPush(w, r)
}
