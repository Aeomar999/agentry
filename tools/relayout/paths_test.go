package main

import "testing"

// The old paths that start other old paths are rewritten only where they
// are whole, in every form a path takes: an import, a ./ or ../ path, and
// a bare word in prose.
func TestReplacePathKeepsLongerNames(t *testing.T) {
	for _, c := range []struct{ old, new, text, want string }{
		{"internal/store", "internal/core/store", `"gitlab.com/dev.chempa/agentry/internal/storetest"`, `"gitlab.com/dev.chempa/agentry/internal/storetest"`},
		{"internal/store", "internal/core/store", `"gitlab.com/dev.chempa/agentry/internal/store"`, `"gitlab.com/dev.chempa/agentry/internal/core/store"`},
		{"internal/web", "internal/x/web", "go test ./internal/webpush/...", "go test ./internal/webpush/..."},
		{"internal/web", "internal/x/web", "go test ./internal/web/...", "go test ./internal/x/web/..."},
		{"internal/workers", "internal/domain/workers", "# internal/workerservice/run.go", "# internal/workerservice/run.go"},
		{"internal/workers", "internal/domain/workers", "# internal/workers/roster.go", "# internal/domain/workers/roster.go"},
		{"internal/settings", "internal/domain/settings", "internal/settingsfile, internal/settings.", "internal/settingsfile, internal/domain/settings."},
		{"internal/store", "internal/core/store", "internal/store-old internal/store_x", "internal/store-old internal/store_x"},
	} {
		if got := replacePath(c.text, c.old, c.new); got != c.want {
			t.Errorf("replacePath(%q, %q) = %q, want %q", c.text, c.old, got, c.want)
		}
	}
}

// A path starts at the start of a word, after the module, or after ./ or
// ../; inside a longer path or name it is not the package's.
func TestReplacePathOnlyWhereAPathStarts(t *testing.T) {
	for text, want := range map[string]string{
		"internal/events":            "internal/core/events",
		"`internal/events`":          "`internal/core/events`",
		"(internal/events/types.go)": "(internal/core/events/types.go)",
		`import catalogue from "../../../internal/events/types.go?raw";`: `import catalogue from "../../../internal/core/events/types.go?raw";`,
		"go run ./internal/events":                                       "go run ./internal/core/events",
		"gitlab.com/dev.chempa/agentry/internal/events":                  "gitlab.com/dev.chempa/agentry/internal/core/events",
		"xinternal/events":                                               "xinternal/events",
		"ui/internal/events":                                             "ui/internal/events",
		"example.com/other/internal/events":                              "example.com/other/internal/events",
		"$(go env X)/internal/events":                                    "$(go env X)/internal/events",
		".internal/events":                                               ".internal/events",
	} {
		if got := replacePath(text, "internal/events", "internal/core/events"); got != want {
			t.Errorf("replacePath(%q) = %q, want %q", text, got, want)
		}
	}
}

// packaging is a bare top-level name and an everyday word: only its uses
// as a path are rewritten.
func TestReplacePathOfABareNameOnlyAsAPath(t *testing.T) {
	for text, want := range map[string]string{
		"go run ./packaging desktop":                 "go run ./tools/packaging desktop",
		"go run ./packaging":                         "go run ./tools/packaging",
		"# packaging/desktop.go packLinuxDesktop":    "# tools/packaging/desktop.go packLinuxDesktop",
		`"packaging/files/install-linux.sh"`:         `"tools/packaging/files/install-linux.sh"`,
		"gitlab.com/dev.chempa/agentry/packaging":    "gitlab.com/dev.chempa/agentry/tools/packaging",
		"// Command packaging makes the downloads":   "// Command packaging makes the downloads",
		`fmt.Fprintln(os.Stderr, "packaging:", err)`: `fmt.Fprintln(os.Stderr, "packaging:", err)`,
		`"internal", "packaging", "spec"`:            `"internal", "packaging", "spec"`,
		"packagingFiles := packaging/x":              "packagingFiles := tools/packaging/x",
		"tools/packaging/main.go":                    "tools/packaging/main.go",
		"the packaging step":                         "the packaging step",
		"./packagings":                               "./packagings",
	} {
		if got := replacePath(text, "packaging", "tools/packaging"); got != want {
			t.Errorf("replacePath(%q) = %q, want %q", text, got, want)
		}
	}
}

// Every occurrence in a text is rewritten, and the text between them kept.
func TestReplacePathRewritesEveryOccurrence(t *testing.T) {
	text := "internal/tasks/a.go and internal/taskstest, then ./internal/tasks/..."
	want := "internal/domain/tasks/a.go and internal/taskstest, then ./internal/domain/tasks/..."
	if got := replacePath(text, "internal/tasks", "internal/domain/tasks"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
