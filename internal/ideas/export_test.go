package ideas

import "testing"

// BeforeMarking runs f between Accept making the task and marking the idea.
func BeforeMarking(t *testing.T, f func()) {
	beforeMarking = f
	t.Cleanup(func() { beforeMarking = func() {} })
}
