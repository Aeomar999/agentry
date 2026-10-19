// Package schema builds the JSON Schemas of MCP tools' arguments and of
// what Claude Code asks you (elicitation), for the core's tools in mcp and
// a feature's alike.
package schema

// Props are an object's properties, by name.
type Props map[string]any

// Object is an object with properties, of which required must be given.
func Object(properties Props, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func Text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
func Texts(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}
func Integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
func Boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
func Enum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}
