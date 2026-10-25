package main

import (
	"strings"
)

// A scenario is one Scenario: block of a feature file: the tag lines
// directly above it, the line it starts on and every line after it up to
// the next scenario's tags.
type scenario struct {
	file  string
	line  int
	title string
	tags  []tag
	body  []line
	// tagsAbove is true when the scenario has no tags and the nearest line
	// above it that is not blank is a tag line: the tags were lost to a
	// blank line.
	tagsAbove bool
}

type tag struct {
	name string
	line int
}

type line struct {
	number int
	text   string // trimmed
}

// parseFeature reads the scenarios out of one feature file's text, and the
// lines that start a Scenario Outline, which this spec does not use. A tag
// line always ends the scenario before it; tags not followed by a scenario,
// such as a feature's own, are dropped.
func parseFeature(file, text string) (scenarios []scenario, outlines []line) {
	var current *scenario
	var pending []tag
	tagsAbove := false // the last line that was not blank was a tag line
	for i, raw := range strings.Split(text, "\n") {
		l := line{number: i + 1, text: strings.TrimSpace(raw)}
		switch {
		case isTagLine(l.text):
			current = nil
			pending = append(pending, tagsOn(l)...)
			tagsAbove = true
		case isScenarioLine(l.text):
			scenarios = append(scenarios, scenario{
				file: file, line: l.number, title: l.text, tags: pending,
				tagsAbove: len(pending) == 0 && tagsAbove,
			})
			current = &scenarios[len(scenarios)-1]
			pending, tagsAbove = nil, false
		case isOutlineLine(l.text):
			outlines = append(outlines, l)
			current, pending, tagsAbove = nil, nil, false
		default:
			pending = nil
			if l.text != "" {
				tagsAbove = false
			}
			if current != nil {
				current.body = append(current.body, l)
			}
		}
	}
	return scenarios, outlines
}

func isTagLine(text string) bool { return strings.HasPrefix(text, "@") }

func isScenarioLine(text string) bool { return strings.HasPrefix(text, "Scenario:") }

func isOutlineLine(text string) bool { return strings.HasPrefix(text, "Scenario Outline:") }

func tagsOn(l line) []tag {
	var tags []tag
	for _, name := range strings.Fields(l.text) {
		if strings.HasPrefix(name, "#") {
			break // a comment after the tags
		}
		tags = append(tags, tag{name: name, line: l.number})
	}
	return tags
}

// thenLines picks the lines a Then rule applies to: each Then, and each
// And or But that continues one, until a Given or When starts something else.
func thenLines(body []line) []line {
	var thens []line
	inThen := false
	for _, l := range body {
		switch stepKeyword(l.text) {
		case "Then":
			inThen = true
			thens = append(thens, l)
		case "And", "But":
			if inThen {
				thens = append(thens, l)
			}
		case "Given", "When":
			inThen = false
		}
	}
	return thens
}

// stepKeyword is the Gherkin keyword a line starts with, or "" for a line
// that is not a step (a comment, a table row, a blank line).
func stepKeyword(text string) string {
	for _, keyword := range []string{"Given", "When", "Then", "And", "But"} {
		if strings.HasPrefix(text, keyword+" ") {
			return keyword
		}
	}
	return ""
}
