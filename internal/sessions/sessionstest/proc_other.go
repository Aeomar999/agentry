//go:build !linux

package sessionstest

// Elsewhere there is no /proc to find this run's processes by: Run checks
// and stops only the sessions whose hosts still answer.
func startedHere(string) []string { return nil }

func killStartedHere(string) {}
