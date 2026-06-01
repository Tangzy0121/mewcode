// Package conversation holds the backend-agnostic message model. A Manager owns
// the in-memory history of rich Messages (text, tool calls, tool results) and
// serializes them to a concrete provider's request shape, so the agent loop and
// providers never touch each other's data formats. Implemented in later tasks.
package conversation
