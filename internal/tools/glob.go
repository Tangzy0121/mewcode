package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type globTool struct{}

// NewGlobTool returns the glob tool.
func NewGlobTool() Tool { return globTool{} }

func (globTool) Name() string { return "glob" }

func (globTool) SideEffecting() bool { return false }

func (globTool) Description() string {
	return "List file paths matching a shell glob pattern. Supports *, ? and [..] within a single path segment (recursive ** is not supported)."
}

func (globTool) Schema() map[string]any {
	return objectSchema(map[string]any{
		"pattern": strProp("Glob pattern, e.g. internal/*/*.go"),
	}, "pattern")
}

func (globTool) Execute(_ context.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err), true
	}
	if args.Pattern == "" {
		return "pattern is required", true
	}
	matches, err := filepath.Glob(args.Pattern)
	if err != nil {
		return fmt.Sprintf("invalid pattern: %v", err), true
	}
	if len(matches) == 0 {
		return fmt.Sprintf("no files match %s", args.Pattern), false
	}
	return strings.Join(matches, "\n"), false
}
