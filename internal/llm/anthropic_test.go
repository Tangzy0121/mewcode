package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mewcode/internal/config"
	"mewcode/internal/conversation"
)

// cannedStream is a trimmed real Messages-API SSE response: streamed text, then
// a tool_use whose input arrives across several input_json_delta chunks.
const cannedStream = `event: message_start
data: {"type":"message_start","message":{"type":"message","role":"assistant","usage":{"input_tokens":472,"output_tokens":2}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the weather."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"location\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" \"San"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" Francisco, CA\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":89}}

event: message_stop
data: {"type":"message_stop"}

`

// T7: the hand-written SSE parser assembles streamed text and a tool call whose
// JSON input is reconstructed from partial_json chunks.
func TestAnthropicStreamParsesTextAndToolUse(t *testing.T) {
	var gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte(cannedStream))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Protocol: config.ProtocolAnthropic, APIKey: "sk-test",
		BaseURL: srv.URL, Model: "claude-opus-4-8", MaxTokens: 1024,
	}
	client, err := NewClient(cfg, "you are a test")
	if err != nil {
		t.Fatal(err)
	}

	conv := conversation.NewManager()
	conv.AddUserText("weather in SF?")
	evs, drainErr := drain(client.Stream(context.Background(), conv, nil))
	if drainErr != nil {
		t.Fatalf("stream error: %v", drainErr)
	}
	if gotKey != "sk-test" {
		t.Errorf("x-api-key header = %q, want sk-test", gotKey)
	}
	if !strings.Contains(gotBody, `"stream":true`) {
		t.Errorf("request body should set stream:true, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"system":"you are a test"`) {
		t.Errorf("request body should carry the system prompt, got: %s", gotBody)
	}

	var text strings.Builder
	var complete *ToolCallComplete
	var end *StreamEnd
	for _, ev := range evs {
		switch e := ev.(type) {
		case TextDelta:
			text.WriteString(e.Text)
		case ToolCallComplete:
			c := e
			complete = &c
		case StreamEnd:
			s := e
			end = &s
		}
	}

	if text.String() != "Let me check the weather." {
		t.Errorf("assembled text = %q", text.String())
	}
	if complete == nil {
		t.Fatal("expected a ToolCallComplete")
	}
	if complete.Name != "get_weather" || complete.ID != "toolu_01" {
		t.Errorf("tool call = %+v", complete)
	}
	if string(complete.Input) != `{"location": "San Francisco, CA"}` {
		t.Errorf("assembled tool input = %s", complete.Input)
	}
	if end == nil || end.StopReason != "tool_use" {
		t.Errorf("StreamEnd = %+v", end)
	}
	if end.Usage.InputTokens != 472 || end.Usage.OutputTokens != 89 {
		t.Errorf("usage = %+v", end.Usage)
	}
}

// debt fix: a transient 503 is retried with backoff and the call eventually
// succeeds, while a non-retryable 401 is returned immediately (no retry).
func TestAnthropicStreamRetriesTransientThenSucceeds(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 { // fail the first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"try later"}}`))
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte(cannedStream))
	}))
	defer srv.Close()

	c := newAnthropicClient(&config.Config{
		Protocol: config.ProtocolAnthropic, APIKey: "sk-test",
		BaseURL: srv.URL, Model: "claude-opus-4-8", MaxTokens: 1024,
	}, "")
	c.baseBackoff = time.Millisecond // keep the test fast

	conv := conversation.NewManager()
	conv.AddUserText("hi")
	evs, err := drain(c.Stream(context.Background(), conv, nil))
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", attempts)
	}
	if len(evs) == 0 {
		t.Error("expected events after a successful retry")
	}
}

func TestAnthropicStreamDoesNotRetryAuthError(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`))
	}))
	defer srv.Close()

	c := newAnthropicClient(&config.Config{
		Protocol: config.ProtocolAnthropic, APIKey: "bad",
		BaseURL: srv.URL, Model: "claude-opus-4-8", MaxTokens: 1024,
	}, "")
	c.baseBackoff = time.Millisecond

	conv := conversation.NewManager()
	conv.AddUserText("hi")
	if _, err := drain(c.Stream(context.Background(), conv, nil)); err == nil {
		t.Fatal("expected an auth error")
	}
	if attempts != 1 {
		t.Errorf("auth error must not retry; got %d attempts", attempts)
	}
}

// T7: a non-2xx response is surfaced as a classified error on the error channel.
func TestAnthropicStreamClassifiesAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Protocol: config.ProtocolAnthropic, APIKey: "bad",
		BaseURL: srv.URL, Model: "claude-opus-4-8", MaxTokens: 1024,
	}
	client, _ := NewClient(cfg, "")
	conv := conversation.NewManager()
	conv.AddUserText("hi")

	_, err := drain(client.Stream(context.Background(), conv, nil))
	if err == nil {
		t.Fatal("expected an error")
	}
	var auth *AuthenticationError
	if !errors.As(err, &auth) {
		t.Errorf("error %T should be *AuthenticationError", err)
	}
}
