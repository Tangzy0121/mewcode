// Command mewcode is a terminal AI coding assistant. With --fake it runs a fully
// offline demo TUI (no API key needed); the real Anthropic backend is wired in
// by tasks T7/T10.
package main

import (
	"context"
	"fmt"
	"os"

	"mewcode/internal/agent"
	"mewcode/internal/config"
	"mewcode/internal/llm"
	"mewcode/internal/permission"
	"mewcode/internal/tools"
	"mewcode/internal/tui"
)

// systemPrompt tells the model what it is and how its tools behave.
const systemPrompt = `You are MewCode, a terminal coding assistant. You work inside the user's current directory and can read, write, and edit files, search the codebase (glob/grep), and run commands.

Tools:
- read_file / glob / grep are read-only; use them freely to understand the code before acting.
- write_file / edit_file change files; edit_file replaces an exact, unique snippet.
- bash runs commands through Windows PowerShell — use PowerShell syntax (e.g. Get-ChildItem, not ls).

Be concise. Inspect before you change. When a tool returns is_error, read the message and adapt instead of repeating the same call.`

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "mewcode:", err)
		os.Exit(1)
	}

	var client llm.Client
	if cfg.Fake {
		client = demoClient{} // offline demo backend, no API key needed
	} else {
		c, err := llm.NewClient(cfg, systemPrompt)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mewcode:", err)
			os.Exit(1)
		}
		client = c
	}

	a := agent.New(client, tools.DefaultRegistry(), permission.NewManager(cfg.Mode))
	if err := tui.Run(context.Background(), a, cfg.Mode); err != nil {
		fmt.Fprintln(os.Stderr, "mewcode:", err)
		os.Exit(1)
	}
}
