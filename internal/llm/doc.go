// Package llm abstracts the model backend behind a single streaming Client
// interface and provides concrete implementations (Anthropic this round, an
// OpenAI-compatible backend reserved for later). It defines the stream event
// sum type and the layered error classification the rest of the program relies
// on. Implemented in later tasks.
package llm
