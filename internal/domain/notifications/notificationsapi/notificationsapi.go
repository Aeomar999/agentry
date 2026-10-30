// Package notificationsapi is the notifications' HTTP API, as the page uses
// it for pushes: whether this device can ask for them, and subscribing a
// browser and unsubscribing it. Web mounts its Routes as it mounts an
// extension's.
package notificationsapi

import (
	"errors"
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/notifications/webpush"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// API holds what the push routes need: the VAPID keys and the browsers
// subscribed, nil when the server keeps none, and the -public-url push
// needs to be https.
type API struct {
	Push      *webpush.Store
	PublicURL string
}

// Routes are the push endpoints, for the page.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/push", Auth: extensions.UI, Handler: a.showPush},
		{Pattern: "POST /api/push/subscriptions", Auth: extensions.UI, Handler: a.subscribePush},
		{Pattern: "DELETE /api/push/subscriptions", Auth: extensions.UI, Handler: a.unsubscribePush},
	}
}

// pushView is whether this device can ask for pushes, as the page shows it.
type pushView struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`    // why not, when not
	PublicKey  string `json:"publicKey,omitempty"` // the VAPID public key the browser subscribes with, when available
	Subscribed int    `json:"subscribed"`          // how many browsers are subscribed
}

// pushAvailable says whether push can work here, and why not: it needs the
// file of subscriptions and an https -public-url.
func (a API) pushAvailable() (bool, string) {
	if a.Push == nil {
		return false, "This server keeps no push subscriptions."
	}
	return webpush.Available(a.PublicURL)
}

func (a API) showPush(w http.ResponseWriter, r *http.Request) {
	available, reason := a.pushAvailable()
	view := pushView{Available: available, Reason: reason}
	if a.Push != nil {
		view.Subscribed = len(a.Push.Subscriptions())
		if available {
			view.PublicKey = a.Push.PublicKey()
		}
	}
	httpapi.Reply(w, view)
}

// subscribePush keeps the browser's PushSubscription, as its toJSON gives
// it. It is refused while push is off, and 400 when malformed.
func (a API) subscribePush(w http.ResponseWriter, r *http.Request) {
	if available, reason := a.pushAvailable(); !available {
		http.Error(w, reason, http.StatusConflict)
		return
	}
	var subscription webpush.Subscription
	if !httpapi.Decode(w, r, &subscription) {
		return
	}
	if err := a.Push.Subscribe(subscription, time.Now()); errors.Is(err, webpush.ErrMalformed) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if err != nil {
		httpapi.Failed(w, err)
		return
	}
	a.showPush(w, r)
}

// unsubscribePush stops the pushes to ?endpoint=, even while push is off.
func (a API) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	if a.Push == nil {
		http.Error(w, "no such subscription", http.StatusNotFound)
		return
	}
	removed, err := a.Push.Unsubscribe(r.URL.Query().Get("endpoint"))
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if !removed {
		http.Error(w, "no such subscription", http.StatusNotFound)
		return
	}
	a.showPush(w, r)
}
