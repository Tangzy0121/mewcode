package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type editTool struct{}

// NewEditTool returns the edit_file tool.
func NewEditTool() Tool { return editTool{} }

func (editTool) Name() string { return "edit_file" }

func (editTool) SideEffecting() bool { return true }

func (editTool) Description() string {
	return "Replace an exact, unique occurrence of old_string with new_string in a file. " +
		"Fails without modifying the file if old_string is missing or appears more than once."
}

func (editTool) Schema() map[string]any {
	return objectSchema(map[string]any{
		"path":       strProp("Path of the file to edit."),
		"old_string": strProp("Exact text to replace. Must occur exactly once in the file."),
		"new_string": strProp("Replacement text. May be empty to delete the old text."),
	}, "path", "old_string", "new_string")
}

func (editTool) Execute(_ context.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err), true
	}
	if args.Path == "" {
		return "path is required", true
	}
	if args.OldString == "" {
		return "old_string must not be empty", true
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return fmt.Sprintf("read %s: %v", args.Path, err), true
	}
	content := string(data)
	switch strings.Count(content, args.OldString) {
	case 0:
		return fmt.Sprintf("old_string not found in %s", args.Path), true
	case 1:
		// unique — safe to replace
	default:
		return fmt.Sprintf("old_string is not unique in %s; add surrounding context to make it unique", args.Path), true
	}
	updated := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(args.Path, []byte(updated), 0o644); err != nil {
		return fmt.Sprintf("write %s: %v", args.Path, err), true
	}
	return fmt.Sprintf("edited %s", args.Path), false
}
