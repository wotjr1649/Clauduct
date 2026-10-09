//go:build !clauduct_agentview_dev

package app

// agentViewDevBuild is true only in a binary built with the clauduct_agentview_dev tag: a
// development build for decision 18's real-backend agent view tests, never installed,
// updated or released. Ordinary builds keep native's agent view off in the foreground.
const agentViewDevBuild = false
