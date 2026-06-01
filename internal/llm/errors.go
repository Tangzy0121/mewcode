package llm

import (
	"fmt"
	"time"
)

// The error layer collapses every provider/SDK/HTTP failure into one of five
// types so the agent loop can react with a single errors.As switch instead of
// understanding each backend's error vocabulary.

// LLMError is the generic, uncategorised backend failure.
type LLMError struct {
	Message string
}

func (e *LLMError) Error() string { return e.Message }

// AuthenticationError signals a rejected or missing credential (HTTP 401).
type AuthenticationError struct {
	Message string
}

func (e *AuthenticationError) Error() string { return "authentication failed: " + e.Message }

// RateLimitError signals throttling (HTTP 429). RetryAfter is the server's
// suggested wait, zero when unspecified.
type RateLimitError struct {
	Message    string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (retry after %s): %s", e.RetryAfter, e.Message)
	}
	return "rate limited: " + e.Message
}

// NetworkError signals a transport-level failure or an idle-timeout on a stalled
// stream, distinct from an error the server reported.
type NetworkError struct {
	Message string
}

func (e *NetworkError) Error() string { return "network error: " + e.Message }

// ContextTooLongError signals the request exceeded the model's context window
// (HTTP 413 or a context-length message). This round it surfaces to the user;
// automatic compaction is out of scope.
type ContextTooLongError struct {
	Message string
}

func (e *ContextTooLongError) Error() string { return "context too long: " + e.Message }
