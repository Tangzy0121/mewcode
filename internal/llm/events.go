package llm

import "encoding/json"

// StreamEvent is a sealed sum type: every event a provider emits during a
// streamed turn implements it. The unexported streamEvent method prevents other
// packages from adding variants, so a type switch in the agent loop can stay
// exhaustive. Thinking/reasoning events are intentionally omitted this round
// (the MVP does not display the model's thinking).
type StreamEvent interface {
	streamEvent()
}

// TextDelta is an incremental chunk of assistant visible text.
type TextDelta struct {
	Text string
}

// ToolCallStart announces the model is beginning a tool call. Its arguments
// arrive afterwards as ToolCallDelta chunks.
type ToolCallStart struct {
	ID   string
	Name string
}

// ToolCallDelta is an incremental chunk of a tool call's JSON arguments,
// associated with a prior ToolCallStart by ID.
type ToolCallDelta struct {
	ID          string
	PartialJSON string
}

// ToolCallComplete carries the fully assembled, parseable arguments for a tool
// call once its argument stream has finished.
type ToolCallComplete struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// StreamEnd is the final event of a turn. StopReason explains why generation
// stopped (e.g. the turn ended or the model wants to call tools); Usage reports
// token consumption.
type StreamEnd struct {
	StopReason string
	Usage      UsageInfo
}

// UsageInfo reports token usage for a single request.
type UsageInfo struct {
	InputTokens  int
	OutputTokens int
}

func (TextDelta) streamEvent()        {}
func (ToolCallStart) streamEvent()    {}
func (ToolCallDelta) streamEvent()    {}
func (ToolCallComplete) streamEvent() {}
func (StreamEnd) streamEvent()        {}
