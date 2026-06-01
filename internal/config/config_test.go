package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv(EnvAnthropicKey, "sk-test")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.Mode != DefaultMode {
		t.Errorf("Mode = %q, want %q", cfg.Mode, DefaultMode)
	}
	if cfg.Protocol != ProtocolAnthropic {
		t.Errorf("Protocol = %q, want %q", cfg.Protocol, ProtocolAnthropic)
	}
	if cfg.MaxTokens != DefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, DefaultMaxTokens)
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want sk-test", cfg.APIKey)
	}
}

func TestLoadMissingKey(t *testing.T) {
	t.Setenv(EnvAnthropicKey, "")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	if !strings.Contains(err.Error(), EnvAnthropicKey) {
		t.Errorf("error %q does not name %s", err, EnvAnthropicKey)
	}
}

func TestLoadInvalidMode(t *testing.T) {
	t.Setenv(EnvAnthropicKey, "sk-test")

	_, err := Load([]string{"--mode", "yolo"})
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "yolo") {
		t.Errorf("error %q should mention the bad value", err)
	}
}

func TestLoadFlagOverride(t *testing.T) {
	t.Setenv(EnvAnthropicKey, "sk-test")

	cfg, err := Load([]string{"--model", "claude-sonnet-4-6", "--mode", "auto"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want override", cfg.Model)
	}
	if cfg.Mode != ModeAuto {
		t.Errorf("Mode = %q, want auto", cfg.Mode)
	}
}
