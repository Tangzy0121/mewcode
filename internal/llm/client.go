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

// NewClient builds the Client for the configured protocol. Anthropic is the
// real backend this round; OpenAI is reserved (interface only) and an unknown
// protocol is rejected here.
func NewClient(cfg *config.Config, systemPrompt string) (Client, error) {
	switch cfg.Protocol {
	case config.ProtocolAnthropic:
		return newAnthropicClient(cfg, systemPrompt), nil
	case config.ProtocolOpenAI:
		return newOpenAIClient(cfg, systemPrompt), nil
	default:
		return nil, fmt.Errorf("unknown protocol %q", cfg.Protocol)
	}
}
