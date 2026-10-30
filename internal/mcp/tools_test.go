package mcp

import (
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/testkit/goldentest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

var update = flag.Bool("update", false, "rewrite testdata/tools.txt from the tools a session lists now")

const rerunTools = "go test ./internal/mcp -run TestTheToolTableIsUpToDate -update"

// testdata/tools.txt is every tool a session lists with every extension
// serve installs shown, and whose it is. mcp has no tools of its own: each
// is a domain's or an extension's, so one added or dropped shows as a diff
// of that file for a reviewer to read.
func TestTheToolTableIsUpToDate(t *testing.T) {
	installed := storetest.Installed()
	s := New(client.Client{}, t.TempDir(), "test", installed)
	s.shown = map[string]bool{}
	owners := map[string][]extensions.Tool{
		"the tasks domain":        s.taskTools().Tools(),
		"the shared files domain": sharedfilesapi.Client{}.Tools(),
	}
	var names []string
	for _, f := range installed {
		s.shown[f.Name] = true
		names = append(names, f.Name)
		if f.Tools != nil {
			owners["the "+f.Name+" extension"] = f.Tools(s.agentry, s.repo)
		}
	}

	var lines []string
	for _, tool := range s.tools() {
		owner := ownerOf(tool.Name, owners)
		if owner == "" {
			t.Errorf("the tool %q is none of the domains' or extensions' tools: a tool belongs in a domain's api package or an extension", tool.Name)
		}
		lines = append(lines, fmt.Sprintf("%-24s %s", tool.Name, owner))
	}
	slices.Sort(lines)
	table := "# Every tool `agentry mcp` lists, and whose it is. Written by\n" +
		"# `" + rerunTools + "`; reviewers read its diff.\n" +
		"# Every extension serve installs is shown: " + strings.Join(names, ", ") + ".\n" +
		strings.Join(lines, "\n") + "\n"
	goldentest.Check(t, filepath.Join("testdata", "tools.txt"), table, *update, rerunTools)
}

// ownerOf is the owner among owners with a tool named name; "" for none.
func ownerOf(name string, owners map[string][]extensions.Tool) string {
	for owner, tools := range owners {
		if slices.ContainsFunc(tools, func(tool extensions.Tool) bool { return tool.Name == name }) {
			return owner
		}
	}
	return ""
}
