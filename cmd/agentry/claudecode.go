package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/mcp"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// What lets Claude Code hear from Agentry between your messages, through
// its hooks (agentry mcp install adds them):
//
//   - When a session starts and with each message you send, what waits for
//     you is added for Claude to read, so it tells you.
//   - After each of Claude's replies, a waiter runs in the background (an
//     asyncRewake hook). When something new waits for you, or a task
//     finishes, it exits 2, which wakes Claude with what it is. One waiter
//     runs per session.
//
// `agentry wait` is the same wait, by hand.

// The hook events agentry claude-hook answers.
const (
	hookContext = "context" // SessionStart and UserPromptSubmit: say what waits
	hookRewake  = "rewake"  // Stop: wait in the background, and wake Claude
)

// waiterLifetime ends a waiter whose session has long gone quiet.
const waiterLifetime = 12 * time.Hour

func runClaudeHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var hook struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
	}
	json.NewDecoder(stdin).Decode(&hook)
	agentry, err := connectFromEnv()
	if err != nil || len(args) != 1 {
		return 0 // Agentry isn't reachable from here: Claude Code goes on without it
	}
	switch args[0] {
	case hookContext:
		return sayWhatWaits(agentry, hook.HookEventName, stdout)
	case hookRewake:
		return waitToWake(agentry, hook.SessionID, stderr)
	}
	return 0
}

// sayWhatWaits adds what waits for you to what Claude reads.
func sayWhatWaits(agentry client.Client, event string, stdout io.Writer) int {
	agentry.Timeout = 3 * time.Second
	waiting, err := mcp.WaitingFor(agentry, "")
	if err != nil || len(waiting) == 0 {
		return 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Agentry: %d waiting for the user:\n", len(waiting))
	for _, w := range waiting {
		fmt.Fprintf(&b, "- %s\n", w.Line)
	}
	b.WriteString("If you haven't told the user about these yet, tell them briefly and offer to deal with them " +
		"(review_approvals, close_epic, answer_question, list_ideas). Don't repeat what you have already told them.")
	reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": b.String()}})
	stdout.Write(reply)
	return 0
}

// waitToWake waits for news (something new waiting for you, or a task
// finished), then exits 2 with it on stderr, which wakes Claude. A waiter already running for
// the session is left to do it.
func waitToWake(agentry client.Client, session string, stderr io.Writer) int {
	release, ok := oneWaiter(session)
	if !ok {
		return 0
	}
	defer release()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, waiterLifetime)
	defer cancel()
	parent := os.Getppid()
	go func() { // Claude Code gone (this waiter handed to another parent): so is the session
		for ctx.Err() == nil && os.Getppid() == parent {
			time.Sleep(10 * time.Second)
		}
		cancel()
	}()
	now, err := mcp.News(agentry)
	if err != nil {
		return 0
	}
	fresh, err := mcp.WaitForNew(ctx, agentry, mcp.Seen(now))
	if err != nil {
		return 0
	}
	fmt.Fprintln(stderr, "Agentry has news for the user:")
	for _, w := range fresh {
		fmt.Fprintf(stderr, "- %s (%s)\n", w.Line, agentry.PageURL("/tasks/"+w.Task))
	}
	fmt.Fprintln(stderr, "Tell the user in a line or two: what finished and what it delivered, and what needs them, "+
		"offering to deal with it (review_approvals, close_epic, answer_question).")
	return 2
}

// oneWaiter claims the session's waiter, by a file with its process id; ok
// is false when a live one has it.
func oneWaiter(session string) (release func(), ok bool) {
	if session == "" {
		session = "default"
	}
	dir, _ := os.UserCacheDir()
	path := filepath.Join(dir, "agentry", "waiters", filepath.Base(session)+".pid")
	os.MkdirAll(filepath.Dir(path), 0o700)
	if held, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(held))); err == nil && alive(pid) {
			return nil, false
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return nil, false
	}
	return func() { os.Remove(path) }, true
}

func alive(pid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}

// runWait waits for news (something new waiting for you, or a task
// finished), prints it and returns.
func runWait(_ []string, _ io.Reader, stdout, stderr io.Writer) int {
	agentry, err := connectFromEnv()
	if err != nil {
		fmt.Fprintln(stderr, "agentry wait:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	now, err := mcp.News(agentry)
	if err != nil {
		fmt.Fprintln(stderr, "agentry wait:", err)
		return 1
	}
	fresh, err := mcp.WaitForNew(ctx, agentry, mcp.Seen(now))
	if err != nil {
		fmt.Fprintln(stderr, "agentry wait:", err)
		return 1
	}
	for _, w := range fresh {
		fmt.Fprintf(stdout, "%s (%s)\n", w.Line, agentry.PageURL("/tasks/"+w.Task))
	}
	return 0
}
