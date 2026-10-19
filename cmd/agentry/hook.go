package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/machines"
)

// runHook is `agentry hook <event>`, which an engine runs with the event's
// JSON on stdin. It posts the call to the server and prints the reply.
//
// Anything short of a clear reply exits 2. A gate hook is installed as
// `... || exit 2`, so the call is blocked: the gate fails closed. A report
// hook is installed as `... || true`, so it never blocks the agent (proof 01).
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentry hook <event>")
		return 2
	}
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return fail(stderr, "reading the hook payload: "+err.Error())
	}
	call := hookcall.Call{
		Task: os.Getenv(hookcall.EnvTask), Turn: os.Getenv(hookcall.EnvTurn),
		Agent: os.Getenv(hookcall.EnvAgent), Engine: os.Getenv(hookcall.EnvEngine),
		Event: args[0], Payload: payload,
	}
	if call.Task == "" || os.Getenv(hookcall.EnvServer) == "" {
		return fail(stderr, "not running in an Agentry session ("+hookcall.EnvTask+" and "+hookcall.EnvServer+" are unset)")
	}
	call.Ending = ending(call)
	call.Plan = plan(call)

	reply, err := post(call)
	if err != nil {
		return fail(stderr, err.Error())
	}
	io.WriteString(stdout, reply.Stdout)
	return reply.ExitCode
}

// ending is what only this machine can tell the server when the call ends
// the turn: the tokens the transcript says were used, and what the agent
// left in its knowledge file. The server may be on another machine.
func ending(call hookcall.Call) *hookcall.Ending {
	engine, ok := engines.Get(call.Engine)
	if !ok {
		return nil
	}
	signals, _ := engine.ParseHook(call.Event, call.Payload)
	for _, signal := range signals {
		if signal.Kind == engines.TurnEnded {
			end := &hookcall.Ending{Tokens: signal.Tokens, TokensUnknown: signal.TokensUnknown, Report: signal.Report}
			if knowledge, err := os.ReadFile(filepath.Join(os.Getenv(hookcall.EnvWorktree), machines.KnowledgeFile)); err == nil {
				text := string(knowledge)
				end.Knowledge = &text
			}
			return end
		}
	}
	return nil
}

// plan is the turn's plan file as the agent has left it, or nil if there is none.
func plan(call hookcall.Call) *string {
	text, err := os.ReadFile(filepath.Join(os.Getenv(hookcall.EnvWorktree), machines.PlanFile(call.Turn)))
	if err != nil {
		return nil
	}
	plan := string(text)
	return &plan
}

// post sends the call. If the engine's hook timeout killed us, the engine
// would fail open, so with a deadline set (shorter than that timeout) we
// give up first, and fail closed (proof 01).
func post(call hookcall.Call) (hookcall.Reply, error) {
	body, _ := json.Marshal(call)
	request, err := http.NewRequest(http.MethodPost, os.Getenv(hookcall.EnvServer)+hookcall.Path, bytes.NewReader(body))
	if err != nil {
		return hookcall.Reply{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(hookcall.TokenHeader, os.Getenv(hookcall.EnvToken))

	client := &http.Client{}
	if deadline, err := time.ParseDuration(os.Getenv(hookcall.EnvDeadline)); err == nil {
		client.Timeout = deadline
	}
	response, err := client.Do(request)
	if err != nil {
		return hookcall.Reply{}, fmt.Errorf("no answer from the Agentry server: %w", err)
	}
	defer response.Body.Close()

	var reply hookcall.Reply
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&reply) != nil {
		return hookcall.Reply{}, fmt.Errorf("bad reply from the Agentry server (HTTP %d)", response.StatusCode)
	}
	return reply, nil
}

func fail(stderr io.Writer, reason string) int {
	fmt.Fprintln(stderr, "Agentry blocked this (fail closed): "+reason)
	return 2
}
