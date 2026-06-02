package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// grepMaxMatches caps fallback output so a broad pattern can't flood the model.
const grepMaxMatches = 200

type grepTool struct{}

// NewGrepTool returns the grep tool.
func NewGrepTool() Tool { return grepTool{} }

func (grepTool) Name() string { return "grep" }

func (grepTool) SideEffecting() bool { return false }

func (grepTool) Description() string {
	return "Search file contents for a regular expression, returning file:line:text matches. " +
		"Uses ripgrep (rg) when installed, otherwise walks the tree with the Go regexp engine."
}

func (grepTool) Schema() map[string]any {
	return objectSchema(map[string]any{
		"pattern": strProp("Regular expression to search for."),
		"path":    strProp("File or directory to search in. Defaults to the current directory."),
	}, "pattern")
}

func (grepTool) Execute(ctx context.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err), true
	}
	if args.Pattern == "" {
		return "pattern is required", true
	}
	root := args.Path
	if root == "" {
		root = "."
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return fmt.Sprintf("invalid pattern: %v", err), true
	}

	if rg, lookErr := exec.LookPath("rg"); lookErr == nil {
		return grepWithRipgrep(ctx, rg, args.Pattern, root)
	}
	return grepWithWalk(re, root)
}

// grepWithRipgrep shells out to ripgrep. rg exits 1 (no matches) is a normal
// empty result, not an error; exit 2 is a real failure.
func grepWithRipgrep(ctx context.Context, rg, pattern, root string) (string, bool) {
	cmd := exec.CommandContext(ctx, rg, "--line-number", "--no-heading", "--color", "never", pattern, root)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "no matches", false
		}
		return fmt.Sprintf("ripgrep failed: %v", err), true
	}
	res := strings.TrimRight(string(out), "\r\n")
	if res == "" {
		return "no matches", false
	}
	return res, false
}

// grepWithWalk is the dependency-free fallback: walk the tree, scan each file
// line by line, collect matches up to the cap.
func grepWithWalk(re *regexp.Regexp, root string) (string, bool) {
	var matches []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip entries we can't stat
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		line := 0
		for sc.Scan() {
			line++
			if re.MatchString(sc.Text()) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", path, line, sc.Text()))
				if len(matches) >= grepMaxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Sprintf("walk failed: %v", walkErr), true
	}
	if len(matches) == 0 {
		return "no matches", false
	}
	return strings.Join(matches, "\n"), false
}
