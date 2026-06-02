package llm

import (
	"context"
	"encoding/json"
	"testing"

	"mewcode/internal/conversation"
)

// drain collects all events from a Stream call and any terminal error.
func drain(out <-chan StreamEvent, errc <-chan error) ([]StreamEvent, error) {
	var evs []StreamEvent
	for ev := range out {
		evs = append(evs, ev)
	}
	select {
	case err := <-errc:
		return evs, err
	default:
		return evs, nil
	}
}

// T5: a fake can script "say something → call one tool → end".
func TestFakeScriptsTextThenToolThenEnd(t *testing.T) {
	fake := NewFakeClient(FakeTurn{Events: []StreamEvent{
		TextDelta{Text: "let me check"},
		ToolCallComplete{ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
		StreamEnd{StopReason: "tool_use"},
	}})

	out, errc := fake.Stream(context.Background(), conversation.NewManager(), nil)
	evs, err := drain(out, errc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(evs), evs)
	}
	if _, ok := evs[0].(TextDelta); !ok {
		t.Errorf("event 0 = %T, want TextDelta", evs[0])
	}
	tc, ok := evs[1].(ToolCallComplete)
	if !ok || tc.Name != "read_file" {
		t.Errorf("event 1 = %+v, want ToolCallComplete read_file", evs[1])
	}
	if end, ok := evs[2].(StreamEnd); !ok || end.StopReason != "tool_use" {
		t.Errorf("event 2 = %+v, want StreamEnd tool_use", evs[2])
	}
}

// T5: successive Stream calls replay successive turns; over-asking surfaces an
// error instead of hanging.
func TestFakeAdvancesTurnsAndReportsExhaustion(t *testing.T) {
	fake := NewFakeClient(
		FakeTurn{Events: []StreamEvent{StreamEnd{StopReason: "tool_use"}}},
		FakeTurn{Events: []StreamEvent{TextDelta{Text: "done"}, StreamEnd{StopReason: "end_turn"}}},
	)
	conv := conversation.NewManager()

	if _, err := drain(fake.Stream(context.Background(), conv, nil)); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	evs, err := drain(fake.Stream(context.Background(), conv, nil))
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	if len(evs) != 2 {
		t.Errorf("turn 2 got %d events, want 2", len(evs))
	}
	// third call: no scripted turn left
	if _, err := drain(fake.Stream(context.Background(), conv, nil)); err == nil {
		t.Error("over-asking should return an error")
	}
}
