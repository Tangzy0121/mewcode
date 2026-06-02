package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeTool struct{}

// NewWriteTool returns the write_file tool.
func NewWriteTool() Tool { return writeTool{} }

func (writeTool) Name() string { return "write_file" }

func (writeTool) SideEffecting() bool { return true }

func (writeTool) Description() string {
	return "Write content to a file, creating parent directories as needed and overwriting any existing file."
}

func (writeTool) Schema() map[string]any {
	return objectSchema(map[string]any{
		"path":    strProp("Path of the file to write."),
		"content": strProp("Full content to write to the file."),
	}, "path", "content")
}

func (writeTool) Execute(_ context.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err), true
	}
	if args.Path == "" {
		return "path is required", true
	}
	if dir := filepath.Dir(args.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Sprintf("create parent dir for %s: %v", args.Path, err), true
		}
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return fmt.Sprintf("write %s: %v", args.Path, err), true
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), false
}
