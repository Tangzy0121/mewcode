package conversation

import (
	"encoding/json"
	"testing"
)

func TestAddAndLen(t *testing.T) {
	m := NewManager()
	if m.Len() != 0 {
		t.Fatalf("new manager Len = %d, want 0", m.Len())
	}
	m.AddUserText("hi")
	m.AddAssistant("sure", []ToolUseBlock{{ID: "t1", Name: "read", Input: json.RawMessage(`{"path":"a"}`)}})
	m.AddToolResults([]ToolResultBlock{{ToolUseID: "t1", Content: "file body", IsError: false}})
	if m.Len() != 3 {
		t.Fatalf("Len = %d, want 3", m.Len())
	}

	msgs := m.GetMessages()
	if msgs[0].Role != RoleUser || msgs[0].Content != "hi" {
		t.Errorf("msg0 = %+v", msgs[0])
	}
	if msgs[1].Role != RoleAssistant || len(msgs[1].ToolUses) != 1 || msgs[1].ToolUses[0].Name != "read" {
		t.Errorf("msg1 = %+v", msgs[1])
	}
	if msgs[2].Role != RoleUser || len(msgs[2].ToolResults) != 1 || msgs[2].ToolResults[0].ToolUseID != "t1" {
		t.Errorf("msg2 = %+v", msgs[2])
	}
}

func TestGetMessagesIsDeepCopy(t *testing.T) {
	m := NewManager()
	m.AddAssistant("x", []ToolUseBlock{{ID: "t1", Name: "read", Input: json.RawMessage(`{"k":1}`)}})

	got := m.GetMessages()
	// Mutate the returned copy aggressively.
	got[0].Content = "TAMPERED"
	got[0].ToolUses[0].Name = "TAMPERED"
	got[0].ToolUses[0].Input[0] = 'X'

	fresh := m.GetMessages()
	if fresh[0].Content != "x" {
		t.Errorf("Content leaked: %q", fresh[0].Content)
	}
	if fresh[0].ToolUses[0].Name != "read" {
		t.Errorf("ToolUses.Name leaked: %q", fresh[0].ToolUses[0].Name)
	}
	if string(fresh[0].ToolUses[0].Input) != `{"k":1}` {
		t.Errorf("Input bytes leaked: %s", fresh[0].ToolUses[0].Input)
	}
}
