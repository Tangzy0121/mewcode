package permission

import (
	"testing"

	"mewcode/internal/config"
)

// C4: the three modes × (read-only / side-effecting) decision matrix.
func TestDecideMatrix(t *testing.T) {
	cases := []struct {
		mode       string
		sideEffect bool
		want       Decision
	}{
		// read-only tools always run, in every mode
		{config.ModePlan, false, Allow},
		{config.ModeDefault, false, Allow},
		{config.ModeAuto, false, Allow},
		// side-effecting tools
		{config.ModePlan, true, Deny},
		{config.ModeDefault, true, Ask},
		{config.ModeAuto, true, Allow},
	}
	for _, c := range cases {
		m := NewManager(c.mode)
		if got := m.Decide("some_tool", c.sideEffect); got != c.want {
			t.Errorf("mode=%s sideEffect=%v: Decide=%v, want %v", c.mode, c.sideEffect, got, c.want)
		}
	}
}

// C4: "always allow" whitelists a tool for the session so later calls skip the
// prompt, while other side-effecting tools still ask.
func TestAlwaysAllowSticksPerTool(t *testing.T) {
	m := NewManager(config.ModeDefault)
	if got := m.Decide("write_file", true); got != Ask {
		t.Fatalf("first call should Ask, got %v", got)
	}
	if run := m.Apply(UserDecision{ToolUseID: "1", Choice: AlwaysAllow}, "write_file"); !run {
		t.Fatal("AlwaysAllow should run the call")
	}
	if got := m.Decide("write_file", true); got != Allow {
		t.Errorf("after AlwaysAllow, write_file should Allow, got %v", got)
	}
	if got := m.Decide("bash", true); got != Ask {
		t.Errorf("a different tool should still Ask, got %v", got)
	}
}

// C4: Apply maps each user choice to run/skip.
func TestApplyChoices(t *testing.T) {
	m := NewManager(config.ModeDefault)
	if run := m.Apply(UserDecision{Choice: AllowOnce}, "bash"); !run {
		t.Error("AllowOnce should run")
	}
	if m.alwaysAllow["bash"] {
		t.Error("AllowOnce must not whitelist the tool")
	}
	if run := m.Apply(UserDecision{Choice: Refuse}, "bash"); run {
		t.Error("Refuse should not run")
	}
}

// C4: mode can be switched at runtime, changing later decisions.
func TestSetModeAtRuntime(t *testing.T) {
	m := NewManager(config.ModePlan)
	if got := m.Decide("bash", true); got != Deny {
		t.Fatalf("plan mode should Deny, got %v", got)
	}
	m.SetMode(config.ModeDefault)
	if got := m.Decide("bash", true); got != Ask {
		t.Errorf("after switch to default, should Ask, got %v", got)
	}
	m.SetMode(config.ModeAuto)
	if got := m.Decide("bash", true); got != Allow {
		t.Errorf("after switch to auto, should Allow, got %v", got)
	}
}
