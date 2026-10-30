package extensions

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// StartJobs starts each extension's Jobs in a goroutine of its own, given the
// Core coreOf makes for it, and returns. A job that panics logs its
// extension, the panic and the stack, and stops; the server and the other
// jobs go on.
func StartJobs(ctx context.Context, list []Extension, coreOf func(Extension) Core) {
	for _, f := range list {
		if f.Jobs != nil {
			go runJob(ctx, f, coreOf(f))
		}
	}
}

func runJob(ctx context.Context, f Extension, c Core) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("agentry: a feature's job panicked and stopped", "feature", f.Name, "panic", r,
				"stack", string(debug.Stack()))
		}
	}()
	f.Jobs(ctx, c)
}
