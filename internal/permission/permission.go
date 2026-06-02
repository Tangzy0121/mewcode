package permission

import (
	"encoding/json"

	"mewcode/internal/config"
)

// Decision is the outcome of gating a tool call.
type Decision int

const (
	// Allow runs the tool without asking.
	Allow Decision = iota
	// Ask suspends the call until the user approves or refuses it.
	Ask
	// Deny refuses the call outright (plan mode for a side-effecting tool).
	Deny
)

// PlanDenialHint is fed back to the model as the tool result when a write/run
// tool is refused in plan mode, telling it (and the user) how to proceed.
const PlanDenialHint = "denied: plan mode is read-only; switch to default or auto mode to run write/edit/bash tools"

// Choice is what the user picks when a call needs approval.
type Choice int

const (
	// AllowOnce runs this single call.
	AllowOnce Choice = iota
	// AlwaysAllow runs this call and auto-allows the same tool for the rest of
	// the session.
	AlwaysAllow
	// Refuse rejects this call.
	Refuse
)

// PendingRequest describes a tool call awaiting the user's decision. The agent
// loop builds it when Decide returns Ask and forwards it to the UI.
type PendingRequest struct {
	ToolUseID string
	ToolName  string
	Input     json.RawMessage
}

// UserDecision is the UI's reply to a PendingRequest.
type UserDecision struct {
	ToolUseID string
	Choice    Choice
}

// Manager holds the current mode and the set of tools the user has chosen to
// always allow this session. It is consulted by the agent loop on a single
// goroutine, matching the conversation manager's single-writer assumption.
type Manager struct {
	mode        string
	alwaysAllow map[string]bool
}

// NewManager starts in the given mode (one of config.Mode*).
func NewManager(mode string) *Manager {
	return &Manager{mode: mode, alwaysAllow: map[string]bool{}}
}

// Mode returns the current mode.
func (m *Manager) Mode() string { return m.mode }

// SetMode switches the mode at runtime (driven by a UI command).
func (m *Manager) SetMode(mode string) { m.mode = mode }

// Decide gates one tool call. Read-only tools always run. Side-effecting tools
// are refused in plan mode, always run in auto mode, and in default mode run
// only if the user previously chose "always allow" for that tool, otherwise
// they need interactive approval.
func (m *Manager) Decide(toolName string, sideEffecting bool) Decision {
	if !sideEffecting {
		return Allow
	}
	switch m.mode {
	case config.ModeAuto:
		return Allow
	case config.ModePlan:
		return Deny
	default: // config.ModeDefault
		if m.alwaysAllow[toolName] {
			return Allow
		}
		return Ask
	}
}

// Apply records the user's decision and returns whether the call should run.
// AlwaysAllow additionally whitelists the tool for the rest of the session.
func (m *Manager) Apply(d UserDecision, toolName string) (run bool) {
	switch d.Choice {
	case AllowOnce:
		return true
	case AlwaysAllow:
		m.alwaysAllow[toolName] = true
		return true
	default: // Refuse
		return false
	}
}
