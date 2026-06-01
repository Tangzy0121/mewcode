// Package agent runs the agentic loop: it sends the conversation to the model,
// consumes the streamed events, dispatches tool calls through the permission
// gate, feeds tool results back into the conversation, and repeats until the
// turn ends. It communicates with the UI only through channels. Implemented in
// later tasks.
package agent
