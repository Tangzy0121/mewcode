package llm

import (
	"context"
	"fmt"

	"mewcode/internal/conversation"
)

// FakeClient is a scripted Client for offline tests and the --fake demo. Each
// call to Stream replays the next FakeTurn: its events are emitted in order on
// the event channel, then, if Err is set, that error is sent on the error
// channel. The agent loop drives one turn per request, so a multi-turn exchange
// ("text → tool call → final text") is expressed as a slice of turns.
type FakeClient struct {
	turns []FakeTurn
	call  int
}

// FakeTurn is one scripted backend response. Events should normally end with a
// StreamEnd; set Err to exercise the loop's error path instead.
type FakeTurn struct {
	Events []StreamEvent
	Err    error
}

// NewFakeClient returns a fake that replays the given turns in order.
func NewFakeClient(turns ...FakeTurn) *FakeClient {
	return &FakeClient{turns: turns}
}

// compile-time proof the fake satisfies the Client interface.
var _ Client = (*FakeClient)(nil)

func (f *FakeClient) Stream(ctx context.Context, _ *conversation.Manager, _ []map[string]any) (<-chan StreamEvent, <-chan error) {
	out := make(chan StreamEvent, 8)
	errc := make(chan error, 1)

	if f.call >= len(f.turns) {
		// Misuse: the loop asked for more turns than were scripted. Surface it
		// rather than hang or silently end.
		errc <- &LLMError{Message: fmt.Sprintf("fake: no scripted turn #%d", f.call+1)}
		close(out)
		return out, errc
	}
	turn := f.turns[f.call]
	f.call++

	go func() {
		defer close(out)
		for _, ev := range turn.Events {
			select {
			case <-ctx.Done():
				errc <- ctx.Err()
				return
			case out <- ev:
			}
		}
		if turn.Err != nil {
			errc <- turn.Err
		}
	}()

	return out, errc
}
