package machines

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// About is everything the page shows about a machine, read when asked: the
// system, what agents will find there, Agentry on it, and its work.
type About struct {
	Host       string    `json:"host"`
	OS         string    `json:"os"`     // as its maker names it: "macOS 15.3", "Ubuntu 24.04.1 LTS"
	Kernel     string    `json:"kernel"` // uname -r
	Arch       string    `json:"arch"`
	CPU        string    `json:"cpu"` // the model, when the machine says
	CPUs       int       `json:"cpus"`
	Memory     uint64    `json:"memory"`     // bytes in all
	MemoryFree uint64    `json:"memoryFree"` // bytes free or reclaimable, when the machine says
	Load       []float64 `json:"load"`       // 1, 5 and 15 minute averages
	BootedAt   time.Time `json:"bootedAt"`
	Disk       uint64    `json:"disk"`     // bytes in all, where the worktrees are
	DiskFree   uint64    `json:"diskFree"` // bytes free there
	Time       time.Time `json:"time"`     // its clock now
	Zone       string    `json:"zone"`

	Tools []Tool `json:"tools"` // engines, git and tmux: what agents run

	Agentry   string   `json:"agentry"`   // its version
	Binary    string   `json:"binary"`    // where it is
	Folder    string   `json:"folder"`    // where its worktrees (and on a worker, its copies of repos) are
	Path      []string `json:"path"`      // the PATH agents get
	Running   []string `json:"running"`   // tasks with a live agent here
	Worktrees []string `json:"worktrees"` // tasks with a worktree here
	Repos     []string `json:"repos"`     // a worker's copies of repos
}

// Tool is one program agents use, and what this machine has of it.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"` // "" when it isn't installed, or didn't say
	Path    string `json:"path"`    // "" when it isn't installed
}

// AboutHere reads About for this machine. worktrees is where its worktrees
// are; repos, on a worker, where its copies of repos are ("" for none).
func AboutHere(worktrees, repos string) About {
	host, _ := os.Hostname()
	self, _ := os.Executable()
	now := time.Now()
	zone, _ := now.Zone()
	a := About{Host: host, OS: osName(), Kernel: output("uname", "-r"), Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
		Time: now, Zone: zone, Agentry: AgentryVersion(), Binary: self, Folder: filepath.Dir(worktrees),
		Path: filepath.SplitList(os.Getenv("PATH")), Running: tasksRunning(),
		Worktrees: folders(worktrees), Repos: folders(repos)}
	a.CPU, a.Memory, a.MemoryFree, a.Load, a.BootedAt = systemFacts()
	a.Disk, a.DiskFree = diskSpace(worktrees)

	a.Tools = tools()
	return a
}

// The tools' versions change rarely and each takes a program run to read,
// so they are kept a few minutes: the page asks often.
var (
	toolsMu   sync.Mutex
	toolsSeen []Tool
	toolsAt   time.Time
)

const toolsKept = 5 * time.Minute

func tools() []Tool {
	toolsMu.Lock()
	defer toolsMu.Unlock()
	if time.Since(toolsAt) < toolsKept {
		return toolsSeen
	}
	toolsSeen = nil
	for _, name := range append(engines.Names(), "git", "tmux") {
		tool := Tool{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			tool.Path = path
			flag := "--version"
			if name == "tmux" {
				flag = "-V"
			}
			tool.Version, _ = versionOf(name, flag)
		}
		toolsSeen = append(toolsSeen, tool)
	}
	toolsAt = time.Now()
	return toolsSeen
}

// tasksRunning are the tasks with a live agent here: every session but the desk's.
func tasksRunning() []string {
	tasks := []string{}
	for _, name := range sessions.Running() {
		if name != "desk" {
			tasks = append(tasks, name)
		}
	}
	return tasks
}

// osName is the OS as its maker names it.
func osName() string {
	if runtime.GOOS == "darwin" {
		return strings.TrimSpace("macOS " + output("sw_vers", "-productVersion"))
	}
	file, err := os.Open("/etc/os-release")
	if err == nil {
		defer file.Close()
		lines := bufio.NewScanner(file)
		for lines.Scan() {
			if value, ok := strings.CutPrefix(lines.Text(), "PRETTY_NAME="); ok {
				return strings.Trim(value, `"`)
			}
		}
	}
	return runtime.GOOS
}

// systemFacts are the CPU model, memory, load and boot time, each read the
// way the machine offers it: /proc on Linux, sysctl on macOS.
func systemFacts() (cpu string, memory, free uint64, load []float64, booted time.Time) {
	if runtime.GOOS == "darwin" {
		cpu = output("sysctl", "-n", "machdep.cpu.brand_string")
		memory, _ = strconv.ParseUint(output("sysctl", "-n", "hw.memsize"), 10, 64)
		free = darwinFreeMemory()
		load = numbers(strings.Trim(output("sysctl", "-n", "vm.loadavg"), "{ }"))
		// kern.boottime: { sec = 1792200000, usec = 0 } ...
		if _, after, ok := strings.Cut(output("sysctl", "-n", "kern.boottime"), "sec = "); ok {
			if seconds, err := strconv.ParseInt(strings.TrimRight(strings.Fields(after)[0], ","), 10, 64); err == nil {
				booted = time.Unix(seconds, 0)
			}
		}
		return
	}
	if info, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(info), "\n") {
			if name, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) == "model name" {
				cpu = strings.TrimSpace(value)
				break
			}
		}
	}
	if info, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(info), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				memory = kb * 1024
			case "MemAvailable:":
				free = kb * 1024
			}
		}
	}
	if info, err := os.ReadFile("/proc/loadavg"); err == nil {
		load = numbers(string(info))
	}
	if info, err := os.ReadFile("/proc/uptime"); err == nil {
		if seconds, err := strconv.ParseFloat(strings.Fields(string(info))[0], 64); err == nil {
			booted = time.Now().Add(-time.Duration(seconds * float64(time.Second))).Truncate(time.Second)
		}
	}
	return
}

// darwinFreeMemory is free and inactive pages from vm_stat: what macOS can
// hand out without swapping.
func darwinFreeMemory() uint64 {
	text := output("vm_stat")
	page := uint64(4096)
	if _, after, ok := strings.Cut(text, "page size of "); ok {
		if size, err := strconv.ParseUint(strings.Fields(after)[0], 10, 64); err == nil {
			page = size
		}
	}
	var pages uint64
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && (name == "Pages free" || name == "Pages inactive" || name == "Pages speculative") {
			n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64)
			pages += n
		}
	}
	return pages * page
}

// diskSpace is the size of the disk dir is on, and what is free on it. A
// folder not made yet is measured by the nearest one that is.
func diskSpace(dir string) (total, free uint64) {
	for dir != "" && dir != "/" {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	var fs syscall.Statfs_t
	if dir == "" || syscall.Statfs(dir, &fs) != nil {
		return 0, 0
	}
	return uint64(fs.Blocks) * uint64(fs.Bsize), uint64(fs.Bavail) * uint64(fs.Bsize)
}

func numbers(text string) []float64 {
	var values []float64
	for _, field := range strings.Fields(text) {
		if value, err := strconv.ParseFloat(field, 64); err == nil && len(values) < 3 {
			values = append(values, value)
		}
	}
	return values
}

// folders are the names of the folders in dir, "" for none.
func folders(dir string) []string {
	names := []string{}
	if dir == "" {
		return names
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// output runs a command for a short answer, "" if it fails.
func output(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
