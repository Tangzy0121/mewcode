package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// readDefaultLimit bounds how many lines a single read returns when the
	// caller gives no limit, so reading a large file cannot flood the context.
	readDefaultLimit = 2000
	// readMaxBytes caps the bytes one read returns regardless of line count, a
	// hard backstop against large files / many long lines.
	readMaxBytes = 256 * 1024
	// readMaxLineBytes truncates any single overlong line (e.g. minified code)
	// so one pathological line cannot blow the budget on its own.
	readMaxLineBytes = 2000
)

type readTool struct{}

// NewReadTool returns the read_file tool.
func NewReadTool() Tool { return readTool{} }

func (readTool) Name() string { return "read_file" }

func (readTool) SideEffecting() bool { return false }

func (readTool) Description() string {
	return fmt.Sprintf("Read a file's UTF-8 text. Optional 1-based offset and limit select a line range "+
		"(default: from line 1, up to %d lines). Output is capped at %d KB; overlong lines and oversized "+
		"reads are truncated with a note telling you how to read the rest.", readDefaultLimit, readMaxBytes/1024)
}

func (readTool) Schema() map[string]any {
	return objectSchema(map[string]any{
		"path":   strProp("Path to the file to read."),
		"offset": map[string]any{"type": "integer", "description": "1-based line number to start from. Defaults to 1."},
		"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to return. Defaults to 2000."},
	}, "path")
}

func (readTool) Execute(_ context.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err), true
	}
	if args.Path == "" {
		return "path is required", true
	}
	offset := args.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 {
		limit = readDefaultLimit
	}

	f, err := os.Open(args.Path)
	if err != nil {
		return fmt.Sprintf("read %s: %v", args.Path, err), true
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var b strings.Builder
	lineNo, collected := 0, 0
	moreLines, bytesCapped := false, false

	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			if lineNo >= offset {
				if collected >= limit {
					moreLines = true
					break
				}
				if len(line) > readMaxLineBytes {
					line = line[:readMaxLineBytes] + "…[line truncated]\n"
				}
				if b.Len()+len(line) > readMaxBytes {
					bytesCapped = true
					break
				}
				b.WriteString(line)
				collected++
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Sprintf("read %s: %v", args.Path, readErr), true
		}
	}

	if collected == 0 {
		if lineNo == 0 {
			return "(empty file)", false
		}
		return fmt.Sprintf("(no lines at offset %d; file has %d lines)", offset, lineNo), false
	}

	result := b.String()
	var notes []string
	if bytesCapped {
		notes = append(notes, fmt.Sprintf("output capped at %d KB", readMaxBytes/1024))
	}
	if moreLines {
		notes = append(notes, fmt.Sprintf("more lines remain — pass offset=%d to continue", offset+collected))
	}
	if len(notes) > 0 {
		result += fmt.Sprintf("\n[read_file: %s]", strings.Join(notes, "; "))
	}
	return result, false
}
