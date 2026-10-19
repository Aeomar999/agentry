package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Each store checks proposals against the catalogue it was opened with, so
// two logs open at once can know different types.
func TestTwoStoresKeepTheirOwnCatalogues(t *testing.T) {
	withIdeas, err := events.NewCatalogue(features.Registration(ideas.Feature()))
	if err != nil {
		t.Fatal(err)
	}
	open := func(catalogue *events.Catalogue) *store.Store {
		s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), catalogue, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	ideasOn, core := open(withIdeas), open(events.Core())
	if ideasOn.Catalogue() != withIdeas {
		t.Error("the store hands out another catalogue than it was opened with")
	}

	suggested := events.Proposal{Stream: "idea-1", Actor: events.Actor{Kind: events.System, ID: "agentry"}, Type: ideas.IdeaSuggested,
		Data: map[string]any{"title": "x", "problem": "p", "proposal": "f", "size": "S", "repo": "/src/app", "task": "task-1"}}
	if _, rejection, err := ideasOn.Propose(suggested); rejection != nil || err != nil {
		t.Errorf("with ideas registered: %v, %v", rejection, err)
	}
	stored, rejection, err := core.Propose(suggested)
	if err != nil {
		t.Fatal(err)
	}
	if rejection == nil || rejection.Check != "shape" || !strings.Contains(rejection.Reason, "stream idea-1 belongs to a feature this build doesn't have") {
		t.Errorf("with the core's types alone: %v, want refused as a foreign stream", rejection)
	}
	if stored.Type != events.ProposalRejected {
		t.Errorf("stored %s, want the rejection", stored.Type)
	}
}
