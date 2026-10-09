//go:build clauduct_agentview_dev

package app

// agentViewDevBuild: see agentview_build.go. This build turns the foreground agent view mode on
// for every launch; every other check -- native identity, routing, session binding,
// confirmation, permissions -- is the product's own.
const agentViewDevBuild = true
