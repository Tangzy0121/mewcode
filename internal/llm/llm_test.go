package llm

import (
	"errors"
	"testing"
	"time"

	"mewcode/internal/config"
)

// Compile-time proof that every event variant satisfies the sealed StreamEvent
// interface. If a variant loses its streamEvent method this fails to build.
var _ = []StreamEvent{
	TextDelta{}, ToolCallStart{}, ToolCallDelta{}, ToolCallComplete{}, StreamEnd{},
}

func TestErrorsAreClassifiable(t *testing.T) {
	cases := []error{
		&LLMError{Message: "boom"},
		&AuthenticationError{Message: "bad key"},
		&RateLimitError{Message: "slow down", RetryAfter: 2 * time.Second},
		&NetworkError{Message: "timeout"},
		&ContextTooLongError{Message: "too big"},
	}
	for _, err := range cases {
		if err.Error() == "" {
			t.Errorf("%T returned empty Error()", err)
		}
	}

	// errors.As must recover the concrete type through the error interface.
	var rl *RateLimitError
	if !errors.As(cases[2], &rl) || rl.RetryAfter != 2*time.Second {
		t.Errorf("errors.As failed to recover RateLimitError, got %+v", rl)
	}
}

func TestNewClientUnknownProtocol(t *testing.T) {
	_, err := NewClient(&config.Config{Protocol: "martian"}, "")
	if err == nil {
		t.Fatal("expected error for unknown protocol")
	}
	if !contains(err.Error(), "martian") {
		t.Errorf("error %q should name the bad protocol", err)
	}
}

func TestNewClientAnthropicNotYetImplemented(t *testing.T) {
	// Until T7 the Anthropic branch is a clearly-marked stub.
	_, err := NewClient(&config.Config{Protocol: config.ProtocolAnthropic}, "")
	if err == nil {
		t.Fatal("expected not-implemented error for anthropic stub")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
