package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mewcode/internal/agent"
	"mewcode/internal/permission"
)

// spyBackend is a test double for the agent loop.
type spyBackend struct {
	events    chan agent.Event
	started   []string
	decisions []permission.UserDecision
	modes     []string
}

func newSpy() *spyBackend { return &spyBackend{events: make(chan agent.Event, 8)} }

func (s *spyBackend) Events() <-chan agent.Event               { return s.events }
func (s *spyBackend) Start(_ context.Context, input string)    { s.started = append(s.started, input) }
func (s *spyBackend) SubmitDecision(d permission.UserDecision) { s.decisions = append(s.decisions, d) }
func (s *spyBackend) SetMode(m string)                         { s.modes = append(s.modes, m) }

func newModel(s *spyBackend) Model {
	m := New(context.Background(), s, "default")
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // sizes the viewport
	return nm.(Model)
}

func step(m Model, msg tea.Msg) Model {
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// T8: streamed text deltas accumulate into the visible reply.
func TestStreamingTextAppears(t *testing.T) {
	m := newModel(newSpy())
	m = step(m, eventMsg{ev: agent.AssistantText{Text: "Hel"}})
	m = step(m, eventMsg{ev: agent.AssistantText{Text: "lo!"}})
	if !strings.Contains(m.View(), "Hello!") {
		t.Errorf("view should show streamed 'Hello!', got:\n%s", m.View())
	}
}

// T8: submitting input starts a turn and marks the model busy.
func TestEnterStartsTurn(t *testing.T) {
	s := newSpy()
	m := newModel(s)
	m.input.SetValue("what is in main.go?")
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(s.started) != 1 || s.started[0] != "what is in main.go?" {
		t.Fatalf("Start not called with input: %v", s.started)
	}
	if !m.busy {
		t.Error("model should be busy after starting a turn")
	}
}

// T9: a tool call shows a card whose status flips from running to ok.
func TestToolCardStatusFlips(t *testing.T) {
	m := newModel(newSpy())
	m = step(m, eventMsg{ev: agent.ToolStarted{ID: "c1", Name: "read_file", Input: `{"path":"main.go"}`}})
	if !strings.Contains(m.View(), "read_file") || !strings.Contains(m.View(), "⏳") {
		t.Errorf("running card missing:\n%s", m.View())
	}
	m = step(m, eventMsg{ev: agent.ToolFinished{ID: "c1", Name: "read_file", Result: "package main", IsError: false}})
	if !strings.Contains(m.View(), "✓") {
		t.Errorf("card should show success glyph:\n%s", m.View())
	}
}

// T9: a permission request shows the prompt; pressing a key answers it.
func TestPermissionPromptAndChoice(t *testing.T) {
	s := newSpy()
	m := newModel(s)
	m = step(m, eventMsg{ev: agent.PermissionAsked{Request: permission.PendingRequest{ToolUseID: "c1", ToolName: "write_file"}}})
	if m.pending == nil || !strings.Contains(m.View(), "allow write_file") {
		t.Fatalf("permission prompt not shown:\n%s", m.View())
	}
	// while pending, the prompt replaces the input box
	if strings.Contains(m.View(), "ctrl+c to quit") {
		t.Error("input footer should be hidden during a permission prompt")
	}
	m = step(m, key("a")) // allow once
	if len(s.decisions) != 1 || s.decisions[0].Choice != permission.AllowOnce {
		t.Fatalf("AllowOnce not submitted: %v", s.decisions)
	}
	if m.pending != nil {
		t.Error("pending should clear after a choice")
	}
}

// T9: /mode switches the permission mode without starting a turn.
func TestModeCommand(t *testing.T) {
	s := newSpy()
	m := newModel(s)
	m.input.SetValue("/mode auto")
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(s.modes) != 1 || s.modes[0] != "auto" {
		t.Fatalf("SetMode not called with auto: %v", s.modes)
	}
	if m.mode != "auto" {
		t.Errorf("display mode = %q, want auto", m.mode)
	}
	if len(s.started) != 0 {
		t.Error("/mode must not start a turn")
	}
}

// scroll fix: when the transcript overflows, new output auto-follows to the
// bottom, and PgUp scrolls back up through history.
func TestViewportScrollsThroughHistory(t *testing.T) {
	m := newModel(newSpy())
	for i := 0; i < 100; i++ { // overflow the 24-row viewport
		m.transcript = append(m.transcript, "history line")
	}
	m.syncViewport()
	if !m.vp.AtBottom() {
		t.Fatal("new content should auto-follow to the bottom")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.vp.AtBottom() {
		t.Error("PgUp should scroll up, away from the bottom")
	}
}

// T8/T9: after the turn finishes the streamed reply lands in the transcript and
// the model is no longer busy.
func TestTurnFinishedCommits(t *testing.T) {
	m := newModel(newSpy())
	m.busy = true
	m = step(m, eventMsg{ev: agent.AssistantText{Text: "all done"}})
	m = step(m, eventMsg{ev: agent.TurnFinished{}})
	if m.busy {
		t.Error("busy should clear on TurnFinished")
	}
	if m.streaming != "" {
		t.Error("streaming buffer should be cleared")
	}
	if !strings.Contains(m.View(), "all done") {
		t.Errorf("committed reply missing:\n%s", m.View())
	}
}
