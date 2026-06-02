package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mewcode/internal/config"
	"mewcode/internal/conversation"
	"mewcode/internal/llm"
	"mewcode/internal/permission"
	"mewcode/internal/tools"
)

// drive runs one turn, collecting all events and auto-answering any permission
// prompt with the given choice. It returns once the turn finishes or errors.
func drive(t *testing.T, a *Agent, input string, choice permission.Choice) []Event {
	t.Helper()
	var evs []Event
	done := make(chan struct{})
	go func() {
		for ev := range a.Events() {
			evs = append(evs, ev)
			switch ev.(type) {
			case PermissionAsked:
				a.SubmitDecision(permission.UserDecision{Choice: choice})
			case TurnFinished, ErrorOccurred:
				close(done)
				return
			}
		}
	}()
	a.RunTurn(context.Background(), input)
	<-done
	return evs
}

func toolCall(id, name, input string) llm.ToolCallComplete {
	return llm.ToolCallComplete{ID: id, Name: name, Input: json.RawMessage(input)}
}

func hasEvent[T Event](evs []Event) bool {
	for _, e := range evs {
		if _, ok := e.(T); ok {
			return true
		}
	}
	return false
}

// C6: the full happy path — text → one tool call → result → final text — runs
// through the loop and the history holds user/assistant/tool_result blocks.
func TestLoopRunsToolAndRecordsHistory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("file body"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := llm.NewFakeClient(
		llm.FakeTurn{Events: []llm.StreamEvent{
			llm.TextDelta{Text: "let me read it"},
			toolCall("c1", "read_file", `{"path":`+strconv(file)+`}`),
			llm.StreamEnd{StopReason: "tool_use"},
		}},
		llm.FakeTurn{Events: []llm.StreamEvent{
			llm.TextDelta{Text: "it says file body"},
			llm.StreamEnd{StopReason: "end_turn"},
		}},
	)
	a := New(fake, tools.DefaultRegistry(), permission.NewManager(config.ModeAuto))

	evs := drive(t, a, "what's in hello.txt?", permission.AllowOnce)

	if !hasEvent[AssistantText](evs) {
		t.Error("expected streamed assistant text")
	}
	if !hasEvent[ToolStarted](evs) || !hasEvent[ToolFinished](evs) {
		t.Error("expected tool start+finish events")
	}
	if !hasEvent[TurnFinished](evs) {
		t.Error("expected TurnFinished")
	}

	hist := a.History()
	// user, assistant(+tooluse), user(toolresult), assistant
	if len(hist) != 4 {
		t.Fatalf("history len = %d, want 4: %+v", len(hist), hist)
	}
	if hist[0].Role != conversation.RoleUser {
		t.Errorf("hist[0] role = %s", hist[0].Role)
	}
	if len(hist[1].ToolUses) != 1 || hist[1].ToolUses[0].Name != "read_file" {
		t.Errorf("hist[1] tool use missing: %+v", hist[1])
	}
	if len(hist[2].ToolResults) != 1 || hist[2].ToolResults[0].IsError {
		t.Errorf("hist[2] tool result wrong: %+v", hist[2])
	}
	if hist[2].ToolResults[0].Content != "file body" {
		t.Errorf("tool result content = %q, want %q", hist[2].ToolResults[0].Content, "file body")
	}
}

// C6: a refused permission yields an is-error tool result and the loop keeps
// going rather than aborting.
func TestLoopContinuesAfterRefusal(t *testing.T) {
	fake := llm.NewFakeClient(
		llm.FakeTurn{Events: []llm.StreamEvent{
			toolCall("c1", "write_file", `{"path":"x.txt","content":"hi"}`),
			llm.StreamEnd{StopReason: "tool_use"},
		}},
		llm.FakeTurn{Events: []llm.StreamEvent{
			llm.TextDelta{Text: "ok, I won't write it"},
			llm.StreamEnd{StopReason: "end_turn"},
		}},
	)
	// default mode → side-effecting write needs approval; we refuse it.
	a := New(fake, tools.DefaultRegistry(), permission.NewManager(config.ModeDefault))

	evs := drive(t, a, "write x.txt", permission.Refuse)

	if !hasEvent[PermissionAsked](evs) {
		t.Error("expected a permission prompt")
	}
	if !hasEvent[TurnFinished](evs) {
		t.Error("loop should finish the turn after a refusal")
	}
	hist := a.History()
	res := hist[2].ToolResults // tool results live on their own user message
	if len(res) != 1 || !res[0].IsError {
		t.Fatalf("expected an is-error refusal tool result, got %+v", hist[2])
	}
}

// C6: a tool that returns is_error is fed back as-is and the loop continues.
func TestLoopContinuesAfterToolError(t *testing.T) {
	fake := llm.NewFakeClient(
		llm.FakeTurn{Events: []llm.StreamEvent{
			toolCall("c1", "read_file", `{"path":"/no/such/file"}`),
			llm.StreamEnd{StopReason: "tool_use"},
		}},
		llm.FakeTurn{Events: []llm.StreamEvent{
			llm.TextDelta{Text: "that file is missing"},
			llm.StreamEnd{StopReason: "end_turn"},
		}},
	)
	a := New(fake, tools.DefaultRegistry(), permission.NewManager(config.ModeAuto))

	evs := drive(t, a, "read a missing file", permission.AllowOnce)

	if !hasEvent[TurnFinished](evs) {
		t.Error("loop should finish despite the tool error")
	}
	res := a.History()[2].ToolResults // tool results live on their own user message
	if len(res) != 1 || !res[0].IsError {
		t.Fatalf("expected an is-error tool result, got %+v", a.History()[2])
	}
}

// strconv quotes a string as a JSON string literal (path may contain backslashes
// on Windows, which must be escaped).
func strconv(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
