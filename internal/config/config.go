// Package config loads MewCode runtime configuration from environment
// variables and command-line flags. It is the single source of truth for the
// model provider, credentials and the startup permission mode; every other
// package receives a ready-to-use *Config rather than reading the environment
// itself.
package config

import (
	"flag"
	"fmt"
	"os"
)

// Protocol identifies which model backend wire format to speak. Only Anthropic
// is implemented this round; OpenAI is reserved so the provider layer can route
// on it without a later config change.
type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
)

// Permission modes. The values are duplicated by the permission package, which
// owns the gating logic; config only validates the startup choice so an invalid
// --mode fails fast instead of surfacing deep in the agent loop.
const (
	ModePlan    = "plan"    // read-only: writes and commands are refused
	ModeDefault = "default" // writes and commands require per-call approval
	ModeAuto    = "auto"    // everything runs without asking
)

// Defaults applied when neither flag nor environment overrides a field.
const (
	DefaultModel     = "claude-opus-4-8"
	DefaultMode      = ModeDefault
	DefaultMaxTokens = 8192
)

// Environment variable names read for credentials.
const (
	EnvAnthropicKey = "ANTHROPIC_API_KEY"
	EnvOpenAIKey    = "OPENAI_API_KEY"
	EnvBaseURL      = "MEWCODE_BASE_URL"
)

// Config is the fully-resolved runtime configuration handed to the rest of the
// program. Once built it is read-only.
type Config struct {
	Protocol  Protocol // which backend wire format to speak
	APIKey    string   // credential for the selected protocol
	BaseURL   string   // optional endpoint override ("" = SDK/default endpoint)
	Model     string   // model id sent on every request
	MaxTokens int      // upper bound on tokens generated per request
	Mode      string   // startup permission mode: plan | default | auto
	Fake      bool     // run the offline demo backend instead of a real API (no key needed)
}

// Load resolves configuration from the given argument list (typically
// os.Args[1:]) and the process environment. Flags override environment which
// overrides built-in defaults. It returns an error — rather than calling
// os.Exit — so callers and tests can decide how to react.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("mewcode", flag.ContinueOnError)
	var (
		model    = fs.String("model", DefaultModel, "model id to use")
		mode     = fs.String("mode", DefaultMode, "permission mode: plan | default | auto")
		protocol = fs.String("protocol", string(ProtocolAnthropic), "model backend: anthropic | openai")
		baseURL  = fs.String("base-url", os.Getenv(EnvBaseURL), "override the API base URL")
		maxTok   = fs.Int("max-tokens", DefaultMaxTokens, "max tokens generated per request")
		fake     = fs.Bool("fake", false, "run the offline demo backend (no API key required)")
	)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{
		Protocol:  Protocol(*protocol),
		BaseURL:   *baseURL,
		Model:     *model,
		MaxTokens: *maxTok,
		Mode:      *mode,
		Fake:      *fake,
	}

	if err := validateMode(cfg.Mode); err != nil {
		return nil, err
	}

	// The offline demo backend needs no credential; skip the key requirement.
	if !cfg.Fake {
		key, err := credential(cfg.Protocol)
		if err != nil {
			return nil, err
		}
		cfg.APIKey = key
	}

	if cfg.MaxTokens <= 0 {
		return nil, fmt.Errorf("max-tokens must be positive, got %d", cfg.MaxTokens)
	}

	return cfg, nil
}

// validateMode rejects any --mode outside the three supported tiers.
func validateMode(mode string) error {
	switch mode {
	case ModePlan, ModeDefault, ModeAuto:
		return nil
	default:
		return fmt.Errorf("invalid mode %q: must be one of %s, %s, %s",
			mode, ModePlan, ModeDefault, ModeAuto)
	}
}

// credential reads the API key for the selected protocol and fails loudly when
// it is missing, naming the exact environment variable to set.
func credential(p Protocol) (string, error) {
	switch p {
	case ProtocolAnthropic:
		key := os.Getenv(EnvAnthropicKey)
		if key == "" {
			return "", fmt.Errorf("missing %s environment variable; set it to your Anthropic API key and retry", EnvAnthropicKey)
		}
		return key, nil
	case ProtocolOpenAI:
		key := os.Getenv(EnvOpenAIKey)
		if key == "" {
			return "", fmt.Errorf("missing %s environment variable; set it to your OpenAI API key and retry", EnvOpenAIKey)
		}
		return key, nil
	default:
		return "", fmt.Errorf("unknown protocol %q: must be %s or %s", p, ProtocolAnthropic, ProtocolOpenAI)
	}
}
