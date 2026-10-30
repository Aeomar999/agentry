package tasks

import (
	"encoding/json"
	"errors"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Archived is the summary an archived task keeps: the "summary" of its
// stream's last task.archived that no task.unarchived follows. It is false
// for a task that isn't archived, and for an erased one, which keeps less. A marker whose summary can't be read
// panics, as an event apply can't take does. store.MarkerOf finds the
// same marker: change the rule in both.
func Archived(stream []events.Event) (Summary, bool) {
	var marker *events.Event
	for i, e := range stream {
		switch e.Type {
		case events.TaskArchived:
			marker = &stream[i]
		case events.TaskUnarchived, events.TaskErased:
			marker = nil
		}
	}
	if marker == nil {
		return Summary{}, false
	}
	return summaryIn(*marker), true
}

// summaryIn reads a marker's summary, as the archiver wrote it from
// SummaryOf. Its id is the stream's, whatever the summary says.
func summaryIn(marker events.Event) Summary {
	summary, err := ReadSummary(marker.Data["summary"])
	if err != nil {
		panic("the marker's summary: " + err.Error())
	}
	summary.ID = marker.Stream
	return summary
}

// ReadSummary reads a summary from a marker's data, or says why it isn't
// one: it must be a JSON object that decodes into a Summary.
func ReadSummary(value any) (Summary, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Summary{}, err
	}
	var summary *Summary // stays nil for null, which a Summary would take as nothing
	if err := json.Unmarshal(data, &summary); err != nil {
		return Summary{}, err
	}
	if summary == nil {
		return Summary{}, errors.New("there is none")
	}
	return *summary, nil
}
