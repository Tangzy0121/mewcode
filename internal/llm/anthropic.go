package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mewcode/internal/config"
	"mewcode/internal/conversation"
)

const anthropicVersion = "2023-06-01"

// Retry policy for transient failures (rate limit, overload, 5xx, network).
const (
	maxAttempts        = 3
	baseBackoffDefault = 500 * time.Millisecond
	maxBackoff         = 8 * time.Second
)

// anthropicClient is the real backend: it speaks the Anthropic Messages API by
// hand over net/http, parses the streamed SSE events, and translates them into
// the backend-agnostic StreamEvent stream the agent loop consumes.
type anthropicClient struct {
	apiKey      string
	endpoint    string
	model       string
	maxTokens   int
	system      string
	baseBackoff time.Duration // first retry wait; field so tests can shrink it
	http        *http.Client
}

var _ Client = (*anthropicClient)(nil)

func newAnthropicClient(cfg *config.Config, systemPrompt string) *anthropicClient {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &anthropicClient{
		apiKey:      cfg.APIKey,
		endpoint:    base + "/v1/messages",
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		system:      systemPrompt,
		baseBackoff: baseBackoffDefault,
		// No client-wide timeout: a stream is long-lived; cancellation rides on
		// the request context instead.
		http: &http.Client{},
	}
}

// apiRequest mirrors the Messages API request body. Opus 4.8 rejects
// temperature/top_p/top_k and budget_tokens, so none are sent.
type apiRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []apiMessage     `json:"messages"`
	Tools     []map[string]any `json:"tools,omitempty"`
	Stream    bool             `json:"stream"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string for plain text, []any of blocks otherwise
}

func (c *anthropicClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan StreamEvent, <-chan error) {
	out := make(chan StreamEvent, 32)
	errc := make(chan error, 1)

	reqBody := apiRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    c.system,
		Messages:  buildMessages(conv.GetMessages()),
		Tools:     toolSchemas,
		Stream:    true,
	}

	go func() {
		defer close(out)

		body, err := json.Marshal(reqBody)
		if err != nil {
			errc <- &LLMError{Message: "encode request: " + err.Error()}
			return
		}
		resp, err := c.send(ctx, body)
		if err != nil {
			errc <- err
			return
		}
		defer resp.Body.Close()
		parseSSE(resp.Body, out, errc)
	}()

	return out, errc
}

// send issues the request, retrying transient failures (429 / 5xx / 529 /
// network) with exponential backoff up to maxAttempts. It returns a live 2xx
// response ready to stream, or a classified terminal error. Mid-stream errors
// are NOT retried here (partial output would already be emitted) — those surface
// from parseSSE instead.
func (c *anthropicClient) send(ctx context.Context, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, &LLMError{Message: "build request: " + err.Error()}
		}
		req.Header.Set("content-type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", anthropicVersion)

		resp, err := c.http.Do(req)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil, &NetworkError{Message: "request cancelled: " + ctx.Err().Error()}
			}
			lastErr = &NetworkError{Message: "request failed: " + err.Error()}
		case resp.StatusCode/100 == 2:
			return resp, nil
		case isRetryableStatus(resp.StatusCode):
			lastErr = classifyHTTPError(resp)
			resp.Body.Close()
		default:
			e := classifyHTTPError(resp)
			resp.Body.Close()
			return nil, e // 4xx (auth, bad request, …) — not retryable
		}

		if attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, &NetworkError{Message: "request cancelled: " + ctx.Err().Error()}
		case <-time.After(c.retryWait(attempt, resp)):
		}
	}
	return nil, lastErr
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		529:                            // overloaded
		return true
	}
	return false
}

// retryWait honours a 429 Retry-After header when present, otherwise backs off
// exponentially (capped at maxBackoff). resp may be nil on a network error.
func (c *anthropicClient) retryWait(attempt int, resp *http.Response) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if ra := parseRetryAfter(resp.Header.Get("retry-after")); ra > 0 {
			if ra > maxBackoff {
				return maxBackoff
			}
			return ra
		}
	}
	w := c.baseBackoff << (attempt - 1)
	if w > maxBackoff {
		return maxBackoff
	}
	return w
}

// buildMessages translates the backend-agnostic history into Anthropic's wire
// shape: assistant turns carry text + tool_use blocks; tool results come back on
// a user turn as tool_result blocks; plain user turns are a string.
func buildMessages(msgs []conversation.Message) []apiMessage {
	out := make([]apiMessage, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.Role == conversation.RoleUser && len(m.ToolResults) > 0:
			blocks := make([]any, 0, len(m.ToolResults))
			for _, r := range m.ToolResults {
				blocks = append(blocks, map[string]any{
					"type":        "tool_result",
					"tool_use_id": r.ToolUseID,
					"content":     r.Content,
					"is_error":    r.IsError,
				})
			}
			out = append(out, apiMessage{Role: "user", Content: blocks})

		case m.Role == conversation.RoleAssistant:
			blocks := make([]any, 0, 1+len(m.ToolUses))
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tu := range m.ToolUses {
				input := tu.Input
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, map[string]any{
					"type": "tool_use", "id": tu.ID, "name": tu.Name, "input": input,
				})
			}
			out = append(out, apiMessage{Role: "assistant", Content: blocks})

		default: // plain user text
			out = append(out, apiMessage{Role: "user", Content: m.Content})
		}
	}
	return out
}

// sseEvent is the union of fields across the Messages API stream event types.
type sseEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type blockState struct {
	id, name string
	buf      strings.Builder
}

// parseSSE reads the event stream line by line and emits StreamEvents. tool_use
// argument JSON arrives as input_json_delta chunks; we accumulate per content
// block and emit a single ToolCallComplete at content_block_stop.
func parseSSE(r io.Reader, out chan<- StreamEvent, errc chan<- error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	tools := map[int]*blockState{}
	var inputTokens, outputTokens int
	var stopReason string

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // skip "event:" lines and blank separators
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "" {
			continue
		}
		var ev sseEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue // tolerate anything unparseable
		}

		switch ev.Type {
		case "message_start":
			inputTokens = ev.Message.Usage.InputTokens
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				tools[ev.Index] = &blockState{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				out <- ToolCallStart{ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				out <- TextDelta{Text: ev.Delta.Text}
			case "input_json_delta":
				if b := tools[ev.Index]; b != nil {
					b.buf.WriteString(ev.Delta.PartialJSON)
					out <- ToolCallDelta{ID: b.id, PartialJSON: ev.Delta.PartialJSON}
				}
			}
		case "content_block_stop":
			if b := tools[ev.Index]; b != nil {
				raw := b.buf.String()
				if raw == "" {
					raw = "{}"
				}
				out <- ToolCallComplete{ID: b.id, Name: b.name, Input: json.RawMessage(raw)}
				delete(tools, ev.Index)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				outputTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			out <- StreamEnd{StopReason: stopReason, Usage: UsageInfo{InputTokens: inputTokens, OutputTokens: outputTokens}}
			return
		case "error":
			errc <- classifySSEError(ev.Error.Type, ev.Error.Message)
			return
		case "ping":
			// keep-alive, ignore
		}
	}
	if err := sc.Err(); err != nil {
		errc <- &NetworkError{Message: "stream read error: " + err.Error()}
	}
}

// classifyHTTPError maps a non-2xx response to one of the layered error types.
func classifyHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &parsed)
	msg := parsed.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthenticationError{Message: msg}
	case http.StatusTooManyRequests:
		return &RateLimitError{Message: msg, RetryAfter: parseRetryAfter(resp.Header.Get("retry-after"))}
	case http.StatusRequestEntityTooLarge:
		return &ContextTooLongError{Message: msg}
	default:
		return &LLMError{Message: fmt.Sprintf("%s (HTTP %d)", msg, resp.StatusCode)}
	}
}

// classifySSEError maps an in-stream error event to a layered error type.
func classifySSEError(errType, msg string) error {
	if msg == "" {
		msg = errType
	}
	switch errType {
	case "rate_limit_error":
		return &RateLimitError{Message: msg}
	case "authentication_error", "permission_error":
		return &AuthenticationError{Message: msg}
	default:
		return &LLMError{Message: msg}
	}
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
