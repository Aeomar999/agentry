package main

// A move is one row of section 4's table: a package's folder today, the
// folder its step puts it in, and the step that moves it. Step 7 moves one
// area of the domain per commit, so its rows name their area too.
// sessions/sessionstest and tasks/taskstest have no rows: they move inside
// their parents' folders.
type move struct {
	step, area string
	old, new   string
}

var moves = []move{
	{"4", "", "internal/storetest", "internal/testkit/storetest"},
	{"4", "", "internal/deeptest", "internal/testkit/deeptest"},
	{"4", "", "internal/speccheck", "tools/speccheck"},
	{"4", "", "packaging", "tools/packaging"},

	{"5", "", "internal/client", "internal/platform/client"},
	{"5", "", "internal/engines", "internal/platform/engines"},
	{"5", "", "internal/exactgob", "internal/platform/exactgob"},
	{"5", "", "internal/hookcall", "internal/platform/hookcall"},
	{"5", "", "internal/hostaddrs", "internal/platform/hostaddrs"},
	{"5", "", "internal/launch", "internal/platform/launch"},
	{"5", "", "internal/pinned", "internal/platform/pinned"},
	{"5", "", "internal/procs", "internal/platform/procs"},
	{"5", "", "internal/schema", "internal/platform/schema"},
	{"5", "", "internal/sessions", "internal/platform/sessions"},
	{"5", "", "internal/terminal", "internal/platform/terminal"},
	{"5", "", "internal/tlsca", "internal/platform/tlsca"},
	{"5", "", "internal/workspace", "internal/platform/workspace"},
	{"5", "", "internal/worktrees", "internal/platform/worktrees"},

	{"6", "", "internal/events", "internal/core/events"},
	{"6", "", "internal/store", "internal/core/store"},
	{"6", "", "internal/current", "internal/core/current"},
	{"6", "", "internal/rules", "internal/core/rules"},
	{"6", "", "internal/httpapi", "internal/core/httpapi"},

	{"7", "agents", "internal/agents", "internal/domain/agents"},
	{"7", "tasks", "internal/tasks", "internal/domain/tasks"},
	{"7", "tasks", "internal/waitlist", "internal/domain/tasks/waitlist"},
	{"7", "workers", "internal/workers", "internal/domain/workers"},
	{"7", "workers", "internal/workerlink", "internal/domain/workers/workerlink"},
	{"7", "workers", "internal/workerservice", "internal/domain/workers/workerservice"},
	{"7", "connections", "internal/connections", "internal/domain/connections"},
	{"7", "policy", "internal/policy", "internal/domain/policy"},
	{"7", "policy", "internal/hooks", "internal/domain/policy/hooks"},
	{"7", "settings", "internal/settings", "internal/domain/settings"},
	{"7", "settings", "internal/settingsfile", "internal/domain/settings/settingsfile"},
	{"7", "sharedfiles", "internal/sharedfiles", "internal/domain/sharedfiles"},
	{"7", "notifications", "internal/webpush", "internal/domain/notifications/webpush"},

	{"8", "", "internal/ideas", "internal/extensions/ideas"},
	{"8", "", "internal/schedules", "internal/extensions/schedules"},

	{"9", "", "internal/coordinator", "internal/runtime/coordinator"},
	{"9", "", "internal/turns", "internal/runtime/turns"},
	{"9", "", "internal/watchdog", "internal/runtime/watchdog"},
	{"9", "", "internal/archive", "internal/runtime/archive"},
	{"9", "", "internal/machines", "internal/runtime/machines"},
	{"9", "", "internal/desk", "internal/runtime/desk"},
}

// An edit is a change a step needs that rewriting paths can't make: a
// relative path whose depth changes (section 7, item 4), or a list of the
// repo's top folders that names one the step empties. file is the file's
// path today; old is today's text, replaced by new wherever it is, after
// the step's git mv and before its paths are rewritten. An edit whose old
// text is gone and whose new text is there is done, so a second run skips
// it.
type edit struct {
	step, area string
	file       string
	old, new   string
}

var edits = []edit{
	// Step 4 empties the root folder packaging/.
	{"4", "", "internal/speccheck/cite.go", `"internal", "packaging", "spec"`, `"internal", "spec"`},
	{"4", "", "spec/README.md", "`internal/`, `packaging/`, `spec/`", "`internal/`, `spec/`"},
	{"4", "", "tools/importrules/rules_test.go", "\t\"packaging\":", "\t\"tools/packaging\":"},
	// packaging goes one folder deeper; its tests read files at the repo root.
	{"4", "", "packaging/desktop_test.go", `filepath.Join("..", path)`, `filepath.Join("..", "..", path)`},
	{"4", "", "packaging/desktop_install_test.go", `t.Chdir("..")`, `t.Chdir("../..")`},
	// events goes one folder deeper.
	{"6", "", "internal/events/fixtures_test.go", `const module = "../.."`, `const module = "../../.."`},
	// web stays, so its path into policy gains domain/.
	{"7", "policy", "internal/web/policy_api_test.go", `"../policy/shipped/`, `"../domain/policy/shipped/`},
	// The rewrite fixes ui/public/sw.js but skips the built page; its copy
	// must stay the same, or the next build changes dist.
	{"7", "notifications", "internal/web/dist/sw.js", "(internal/webpush)", "(internal/domain/notifications/webpush)"},
}

// allowed are the files of a moved package whose `..` or Chdir is not a
// path to fix (section 7, item 4's second list), each with why. It names
// files, not lines, so an edit to one doesn't stop a step; a new file with
// a hit does.
var allowed = map[string]string{
	"internal/policy/decide.go":                  "comments on how .. is cleaned",
	"internal/policy/reach.go":                   "comments on how .. is cleaned",
	"internal/policy/shell.go":                   "comments on .., and splitting a shell range on ..",
	"internal/sharedfiles/folder.go":             "refuses names that escape the folder",
	"internal/sharedfiles/blobs_test.go":         "tests that a name can't escape the folder",
	"internal/sharedfiles/sharedfiles_test.go":   "tests that a name can't escape the folder",
	"internal/worktrees/repo_test.go":            "tests that a file can't be made outside the worktree",
	"internal/workspace/workspace_test.go":       "tests that a path can't leave the workspace",
	"internal/coordinator/handoff_brief_test.go": "a brief's text that names a commit range",
	"internal/speccheck/sources.go":              "compares a path's parts with \"..\" to refuse paths out of the repo",
	"internal/speccheck/check_test.go":           "its ../ paths are fixtures' and its Chdirs go into temp folders; tools/speccheck is as deep as internal/speccheck",
	"packaging/check_test.go":                    "Chdirs into a temp repo",
}

// areas are step 7's areas, in the order the doc moves them.
var areas = []string{"agents", "tasks", "workers", "connections", "policy", "settings", "sharedfiles", "notifications"}

// movesOf gives the rows of a step, or of one area of step 7; an empty
// area means the whole step.
func movesOf(step, area string) []move {
	var rows []move
	for _, m := range moves {
		if m.step == step && (area == "" || m.area == area) {
			rows = append(rows, m)
		}
	}
	return rows
}

// editsOf gives the edits of a step, or of one area of step 7.
func editsOf(step, area string) []edit {
	var rows []edit
	for _, e := range edits {
		if e.step == step && (area == "" || e.area == area) {
			rows = append(rows, e)
		}
	}
	return rows
}
