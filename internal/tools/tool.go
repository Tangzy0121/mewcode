package tools

import (
	"context"
	"encoding/json"
)

// Tool is one capability the model can invoke. Name and Description are shown to
// the model; Schema is the JSON Schema for the tool's arguments. Execute parses
// the raw JSON input and returns the textual result plus an isError flag, which
// the agent loop carries back to the model as a ToolResultBlock so the model can
// self-correct on failure rather than assume success. Side-effecting tools do
// not check permission themselves — the agent loop gates them before dispatch.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	// SideEffecting reports whether the tool changes state (writes files, runs
	// commands) and so must pass permission before it runs. Read-only tools
	// return false. This is the single source of truth the permission layer
	// consults, so the classification cannot drift from the tools.
	SideEffecting() bool
	Execute(ctx context.Context, input json.RawMessage) (content string, isError bool)
}

// objectSchema builds a JSON Schema object node with the given properties and
// required keys, so each tool declares its arguments in one line.
func objectSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// strProp is a string-typed schema property with a description.
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
