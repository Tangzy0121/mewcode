package llm

import (
	"context"
	"fmt"

	"mewcode/internal/config"
	"mewcode/internal/conversation"
)

// Client is the single streaming abstraction over a model backend. Stream sends
// the conversation plus the available tool schemas and returns two channels: a
// buffered stream of events and a separate error channel. Exactly one terminal
// signal arrives per call — a StreamEnd on success, or one classified error on
// the error channel. The provided context cancels the in-flight request.
//
// Tool schemas are passed as raw maps so the llm package stays decoupled from
// the tools package; the caller (agent loop) supplies them from the registry.
type Client interface {
	Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan StreamEvent, <-chan error)
}

// NewClient builds the Client for the configured protocol. The concrete
// implementations are wired in by later tasks: the fake backend in T5, the
// Anthropic backend in T7. Until then a valid protocol returns a clear
// not-implemented error, while an unknown protocol is rejected here.
func NewClient(cfg *config.Config, systemPrompt string) (Client, error) {
	switch cfg.Protocol {
	case config.ProtocolAnthropic:
		return nil, fmt.Errorf("anthropic backend not implemented yet (task T7)")
	case config.ProtocolOpenAI:
		return nil, fmt.Errorf("openai backend not implemented yet (out of scope this round)")
	default:
		return nil, fmt.Errorf("unknown protocol %q", cfg.Protocol)
	}
}
