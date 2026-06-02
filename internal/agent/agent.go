package agent

import (
	"context"
	"strings"

	"mewcode/internal/conversation"
	"mewcode/internal/llm"
	"mewcode/internal/permission"
	"mewcode/internal/tools"
)

// Event is a sealed sum type of everything the loop reports to the UI. The
// unexported event method keeps the variants closed so a UI type switch stays
// exhaustive.
type Event interface{ event() }

// AssistantText is an incremental chunk of the assistant's visible reply.
type AssistantText struct{ Text string }

// ToolStarted announces a tool is about to run (already permitted).
type ToolStarted struct {
	ID, Name, Input string
}

// ToolFinished reports a tool's outcome.
type ToolFinished struct {
	ID, Name, Result string
	IsError          bool
}

// PermissionAsked asks the UI to approve or refuse a side-effecting call. The
// loop blocks until SubmitDecision delivers the answer.
type PermissionAsked struct{ Request permission.PendingRequest }

// TurnFinished signals the user's turn fully completed.
type TurnFinished struct{}

// ErrorOccurred reports a backend/turn error. The agent stays alive for the
// next user input.
type ErrorOccurred struct{ Err error }

func (AssistantText) event()   {}
func (ToolStarted) event()     {}
func (ToolFinished) event()    {}
func (PermissionAsked) event() {}
func (TurnFinished) event()    {}
func (ErrorOccurred) event()   {}

// Agent owns the conversation and drives one user turn at a time. It talks to
// the UI only through the events and decisions channels — it never touches UI
// state — satisfying the loop/UI decoupling requirement.
type Agent struct {
	client llm.Client
	tools  *tools.Registry
	perm   *permission.Manager
	conv   *conversation.Manager

	events    chan Event
	decisions chan permission.UserDecision
}

// New builds an agent over the given collaborators.
func New(client llm.Client, reg *tools.Registry, perm *permission.Manager) *Agent {
	return &Agent{
		client:    client,
		tools:     reg,
		perm:      perm,
		conv:      conversation.NewManager(),
		events:    make(chan Event, 16),
		decisions: make(chan permission.UserDecision, 1),
	}
}

// Events is the stream the UI consumes.
func (a *Agent) Events() <-chan Event { return a.events }

// SubmitDecision delivers the user's answer to a pending PermissionAsked.
func (a *Agent) SubmitDecision(d permission.UserDecision) { a.decisions <- d }

// SetMode switches the permission mode at runtime (UI command).
func (a *Agent) SetMode(mode string) { a.perm.SetMode(mode) }

// History returns a copy of the conversation so far (for tests and inspection).
func (a *Agent) History() []conversation.Message { return a.conv.GetMessages() }

// Start runs one user turn on its own goroutine so a UI can stay responsive
// while events stream back over Events().
func (a *Agent) Start(ctx context.Context, userInput string) {
	go a.RunTurn(ctx, userInput)
}

// RunTurn processes one user message to completion, emitting events as it goes
// and ending with TurnFinished (or ErrorOccurred). Run it on its own goroutine;
// the caller drives permission answers via SubmitDecision.
func (a *Agent) RunTurn(ctx context.Context, userInput string) {
	a.conv.AddUserText(userInput)

	for {
		text, toolCalls, err := a.streamOnce(ctx)
		if err != nil {
			a.events <- ErrorOccurred{Err: err}
			return
		}
		a.conv.AddAssistant(text, toolCalls)
		if len(toolCalls) == 0 {
			break // the model produced a final answer; turn is done
		}

		results := make([]conversation.ToolResultBlock, 0, len(toolCalls))
		for _, tc := range toolCalls {
			results = append(results, a.handleToolCall(ctx, tc))
		}
		a.conv.AddToolResults(results)
		// loop: feed the results back to the model for the next step
	}

	a.events <- TurnFinished{}
}

// streamOnce runs a single backend request, accumulating the assistant text and
// any tool calls. The provider is responsible for honouring ctx and for closing
// the event channel when the response ends.
func (a *Agent) streamOnce(ctx context.Context) (string, []conversation.ToolUseBlock, error) {
	out, errc := a.client.Stream(ctx, a.conv, a.tools.Definitions())

	var text strings.Builder
	var toolCalls []conversation.ToolUseBlock
	for ev := range out {
		switch e := ev.(type) {
		case llm.TextDelta:
			text.WriteString(e.Text)
			a.events <- AssistantText{Text: e.Text}
		case llm.ToolCallComplete:
			toolCalls = append(toolCalls, conversation.ToolUseBlock{ID: e.ID, Name: e.Name, Input: e.Input})
		}
		// ToolCallStart/Delta are assembled into ToolCallComplete by the
		// provider; StreamEnd carries no payload the loop needs here.
	}
	// The stream is drained; surface any terminal error.
	select {
	case err := <-errc:
		if err != nil {
			return "", nil, err
		}
	default:
	}
	return text.String(), toolCalls, nil
}

// handleToolCall gates one call through permission, runs it if permitted, and
// returns the tool result to feed back to the model. A denial or refusal yields
// an is-error result so the model can adapt rather than assume success.
func (a *Agent) handleToolCall(ctx context.Context, tc conversation.ToolUseBlock) conversation.ToolResultBlock {
	tool, known := a.tools.Get(tc.Name)
	sideEffecting := known && tool.SideEffecting()

	switch a.perm.Decide(tc.Name, sideEffecting) {
	case permission.Deny:
		res := conversation.ToolResultBlock{ToolUseID: tc.ID, Content: permission.PlanDenialHint, IsError: true}
		a.events <- ToolFinished{ID: tc.ID, Name: tc.Name, Result: res.Content, IsError: true}
		return res
	case permission.Ask:
		a.events <- PermissionAsked{Request: permission.PendingRequest{ToolUseID: tc.ID, ToolName: tc.Name, Input: tc.Input}}
		decision := <-a.decisions
		if !a.perm.Apply(decision, tc.Name) {
			res := conversation.ToolResultBlock{ToolUseID: tc.ID, Content: "denied: the user refused this tool call", IsError: true}
			a.events <- ToolFinished{ID: tc.ID, Name: tc.Name, Result: res.Content, IsError: true}
			return res
		}
		// approved — fall through to run
	case permission.Allow:
		// run
	}

	a.events <- ToolStarted{ID: tc.ID, Name: tc.Name, Input: string(tc.Input)}
	content, isErr := a.tools.Dispatch(ctx, tc.Name, tc.Input)
	a.events <- ToolFinished{ID: tc.ID, Name: tc.Name, Result: content, IsError: isErr}
	return conversation.ToolResultBlock{ToolUseID: tc.ID, Content: content, IsError: isErr}
}
