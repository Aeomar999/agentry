package engines

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The two kinds of hook command Agentry installs (proof 01):
//
//	a gate fails closed: if the hook crashes or exits oddly, `|| exit 2` blocks the call
//	a report never blocks: `|| true`, because exit 2 on UserPromptSubmit blocks the prompt
func gateHook(hookCommand, event string) map[string]any {
	return map[string]any{"type": "command", "command": hookCommand + " " + event + " || exit 2", "timeout": GateTimeoutSeconds}
}

func reportHook(hookCommand, event string) map[string]any {
	return map[string]any{"type": "command", "command": hookCommand + " " + event + " || true"}
}

// hookSettings is the "hooks" block both engines read: the gate events get a
// gate hook, every other event a report hook.
func hookSettings(hookCommand string, gates, reports []string) map[string]any {
	hooks := map[string]any{}
	for _, event := range gates {
		hooks[event] = []any{map[string]any{"matcher": "*", "hooks": []any{gateHook(hookCommand, event)}}}
	}
	for _, event := range reports {
		hooks[event] = []any{map[string]any{"matcher": "*", "hooks": []any{reportHook(hookCommand, event)}}}
	}
	return map[string]any{"hooks": hooks}
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return os.WriteFile(path, encoded, 0o644)
}
