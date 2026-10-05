package app

import (
	"errors"
	"strings"
)

// Which Anthropic-hosted paths native keeps in a Clauduct session (#301). Both profiles go
// in the child's --settings, which outranks user, project and local settings files.
//
// default (the owner's decision, 2026-10-05) keeps native's auto-update, the WebFetch domain
// check and the MCP registry list, and turns off the paths that act on a claude.ai account:
// claude.ai connectors and DesignSync (claude.ai/design). It is not "no Anthropic traffic".
//
// strict also turns off native's nonessential traffic (auto-update, the MCP registry list,
// release notes), the WebFetch domain check -- and with it Anthropic's blocklist of domains
// WebFetch must not read, a security trade-off the user takes -- and the Artifact tools.
// Native's official-marketplace auto-install is turned off by a variable that, the first
// time, records the refusal in native's own config for good; Clauduct does not change that
// state for the user, so strict starts only when the user has set the variable.
const (
	boundaryDefault = "default"
	boundaryStrict  = "strict"

	nonessentialTraffic = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"
	marketplaceInstall  = "CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL"
)

// Measured 2.1.289 (#301): a settings deny removes the tool from what the model is offered,
// joins the user's own deny rules, and a user allow does not bring it back. The same through
// --disallowedTools would mean writing into argv, where that option takes several values.
var (
	defaultDenied = []string{"DesignSync"}
	strictDenied  = []string{"Artifact", "ArtifactCheck", "ArtifactComments", "ArtifactData", "DesignSync"}
)

var errStrictMarketplace = errors.New("BOUNDARY_PROFILE_STRICT: boundary_profile is strict but " + marketplaceInstall +
	" is not set to a value native reads as on (1, true, yes, on). Native records it in its own config the first time it sees it, so the official marketplace stays off for later sessions too; " +
	"Clauduct does not make that change for you. Set " + marketplaceInstall + "=1 in the environment you start clauduct from, or use boundary_profile \"default\"")

// boundarySettings fills the profile's part of the child's settings.
func (config ClauductSettings) boundarySettings(s *childSettings) {
	s.DisableClaudeAiConnectors = true
	denied := defaultDenied
	if config.BoundaryProfile == boundaryStrict {
		s.SkipWebFetchPreflight = true
		denied = strictDenied
	}
	s.Permissions = &childPermissions{Deny: denied}
}

// strictReady refuses strict before anything starts unless the user set the marketplace
// variable to a value native reads as on, and returns the settings with that value kept: the
// session's requirements re-assert it so a settings file's env, which outranks the process
// environment, cannot quietly undo the user's choice. Clauduct never chooses the value.
func (config ClauductSettings) strictReady(env map[string]string) (ClauductSettings, error) {
	if config.BoundaryProfile != boundaryStrict {
		return config, nil
	}
	for name, value := range env {
		if strings.EqualFold(name, marketplaceInstall) && nativeTruthy(value) {
			config.marketplace = value
			return config, nil
		}
	}
	return config, errStrictMarketplace
}

// nativeTruthy is native 2.1.289's reading of a boolean environment variable (its env schema's
// bool: trimmed, lower-cased, one of 1/true/yes/on).
func nativeTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
