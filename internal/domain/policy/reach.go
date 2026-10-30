package policy

import (
	"path/filepath"
	"slices"
	"strings"
)

// mayReach: path, which may hold the shell's glob characters, could name
// protected or something inside it. With holders, a folder that holds
// protected counts too, as it does for a program that reads folders whole.
//
// Each name in path is matched against the one at the same depth in
// protected. Past a "**" or a brace the depths can't be lined up, so the
// path reaches, as it does when a pattern can't be read: if unsure, refuse.
func mayReach(path, protected string, holders bool) bool {
	names, protectedNames := splitPath(path), splitPath(protected)
	for i, want := range protectedNames {
		if i == len(names) {
			return holders
		}
		name := names[i]
		if strings.Contains(name, "**") || strings.ContainsAny(name, "{}") {
			return true
		}
		if !nameMayBe(name, want, i == len(protectedNames)-1) {
			return false
		}
	}
	return true
}

// splitPath is a cleaned absolute path's names; the root has none.
func splitPath(path string) []string {
	return strings.Split(strings.TrimSuffix(path, string(filepath.Separator)), string(filepath.Separator))
}

// nameMayBe: a name in a path, perhaps a pattern, may be the protected name
// want. The last protected name also covers the files a database keeps
// beside it: agentry.db-wal, agentry.db-shm. Case is ignored, as macOS's
// disks and bash's nocaseglob ignore it.
func nameMayBe(name, want string, last bool) bool {
	name, want = strings.ToLower(name), strings.ToLower(want)
	if last && strings.HasPrefix(name, want+"-") || hasUnmatchablePattern(name) {
		return true
	}
	candidates := []string{want}
	if last {
		candidates = append(candidates, want+"-wal", want+"-shm", want+"-journal")
	}
	pattern := strings.ReplaceAll(name, "[!", "[^") // the shell's [!x] is Go's [^x]
	for _, candidate := range candidates {
		if matched, err := filepath.Match(pattern, candidate); matched || err != nil {
			return true
		}
	}
	return false
}

// Patterns bash knows and filepath.Match doesn't: classes such as [[:alpha:]],
// [[=t=]] and [[.t.]], which Match reads as plain classes and so never
// matches, and extglob's @(x), +(x), !(x), ?(x) and *(x).
var unmatchablePatterns = []string{"[:", "[=", "[.", "@(", "+(", "!(", "?(", "*("}

// hasUnmatchablePattern: name holds a pattern that can't be matched here, so
// it may be anything.
func hasUnmatchablePattern(name string) bool {
	return slices.ContainsFunc(unmatchablePatterns, func(pattern string) bool { return strings.Contains(name, pattern) })
}

// Programs known not to read what is in a folder they are given, so a folder
// that holds Agentry's files may be given to them. Every other program may
// read it whole (tar c ~, diff -r ~/.agentry x, fd -x cat), and so may one
// whose name can't be known. Each lists the options that make it name every
// file below the folder; GNU takes a long option's unambiguous start, --rec.
var foldersLeftAlone = map[string][]string{
	"ls": {"-R", "--recursive"}, // names a folder's entries, not what is in them
	"du": {"-a", "--all"},       // adds up sizes; it names files only with -a
	"df": nil,                   // the free space of the disk a path is on
	// cd, pushd and popd move the shell; pwd, realpath and readlink print
	// where a path leads, and dirname and basename only cut its text.
	"cd": nil, "pushd": nil, "popd": nil, "pwd": nil,
	"realpath": nil, "readlink": nil, "dirname": nil, "basename": nil,
	"stat": nil, "test": nil, "[": nil, // a path's type, size and permissions
	"mkdir": nil,                                           // makes a folder and, with -p, those above it
	"echo":  nil, "printf": nil, "true": nil, "false": nil, // never open their arguments
}

// readsFoldersWhole: the command's program may read what is in a folder it
// is given. Only a program named exactly is known: ./ls may be anything.
func readsFoldersWhole(words []string) bool {
	if len(words) == 0 {
		return false
	}
	options, known := foldersLeftAlone[words[0]]
	if !known {
		return true
	}
	return slices.ContainsFunc(words[1:], func(word string) bool {
		return slices.ContainsFunc(options, func(option string) bool { return givesOption(word, option) })
	})
}

// givesOption: word is option, alone or among clustered short options (-laR),
// or the start of a long one (--recur).
func givesOption(word, option string) bool {
	if strings.HasPrefix(option, "--") {
		return len(word) > 2 && strings.HasPrefix(option, word)
	}
	return len(word) > 1 && word[0] == '-' && word[1] != '-' && strings.Contains(word[1:], option[1:])
}

// loopsBack: path, absolute but not cleaned, leads back into the file system
// through a process's working directory, root or open files, so it may reach
// anything. Each start of the path is cleaned on its own, as the kernel reads
// a ".." before it but not one after it. An open file alone is only what the
// shell already opened; a path that goes on below it reads it as a folder.
func loopsBack(path string) bool {
	names := strings.Split(followDevLinks(path), string(filepath.Separator))
	for end := range names {
		start := splitPath(filepath.Clean("/" + strings.Join(names[:end+1], "/")))
		if isProcessLink(start, "cwd", "root") {
			return true
		}
		goesOn := slices.ContainsFunc(names[end+1:], func(name string) bool { return name != "" })
		if goesOn && isOpenFile(start) {
			return true
		}
	}
	return false
}

// devLinks are /dev's links into a process's open files. The kernel follows
// them before a ".." after them, so /dev/fd/.. is /proc/self, not /dev.
var devLinks = map[string]string{
	"/dev/fd":     "/proc/self/fd",
	"/dev/stdin":  "/proc/self/fd/0",
	"/dev/stdout": "/proc/self/fd/1",
	"/dev/stderr": "/proc/self/fd/2",
}

// followDevLinks: path, absolute but not cleaned, with a start that is one
// of devLinks put as where it leads.
func followDevLinks(path string) string {
	names := strings.Split(path, string(filepath.Separator))
	for end := range names {
		start := filepath.Clean("/" + strings.Join(names[:end+1], "/"))
		if target, isLink := devLinks[start]; isLink {
			return target + "/" + strings.Join(names[end+1:], "/")
		}
	}
	return path
}

// isOpenFile: names, a cleaned path's, are a file a process has open:
// /proc/<process>/fd/<n> or a thread's.
func isOpenFile(names []string) bool {
	n := len(names)
	return n > 4 && isProcessLink(names[:n-1], "fd")
}

// isProcessLink: names, a cleaned path's, are one of a process's links in
// /proc, /proc/<process>/<link>, or one of its threads',
// /proc/<process>/task/<thread>/<link>.
func isProcessLink(names []string, links ...string) bool {
	if len(names) < 4 || names[1] != "proc" || !mayBeProcess(names[2]) {
		return false
	}
	link := names[3]
	if len(names) == 6 && names[3] == "task" && mayBeProcess(names[4]) {
		link = names[5]
	} else if len(names) != 4 {
		return false
	}
	return slices.ContainsFunc(links, func(want string) bool { return nameMayBe(link, want, false) })
}

// mayBeProcess: a name in /proc, perhaps a pattern, may be a process's.
func mayBeProcess(name string) bool {
	return name == "self" || name == "thread-self" || strings.Trim(name, "0123456789") == "" ||
		strings.ContainsAny(name, "*?[{") || hasUnmatchablePattern(name)
}
