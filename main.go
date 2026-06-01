// Command mewcode is a terminal AI coding assistant. This entry point currently
// loads and validates configuration; the agent loop and TUI are wired in by
// later tasks.
package main

import (
	"fmt"
	"os"

	"mewcode/internal/config"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "mewcode:", err)
		os.Exit(1)
	}

	// Placeholder until the TUI is wired in (task T10). Proves the config layer
	// resolves and is observable end-to-end.
	fmt.Printf("mewcode ready: protocol=%s model=%s mode=%s max-tokens=%d\n",
		cfg.Protocol, cfg.Model, cfg.Mode, cfg.MaxTokens)
}
