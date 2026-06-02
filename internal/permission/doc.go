// Package permission implements the three-tier gating model (plan, default,
// auto). It decides, for a given tool and the current mode, whether a call is
// allowed, refused, or needs interactive user approval, and carries the pending
// request and the user's decision between the agent loop and the UI.
package permission
