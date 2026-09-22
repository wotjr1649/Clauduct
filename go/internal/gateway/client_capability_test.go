package gateway

import (
	"strings"
	"testing"
)

func TestMissingRequestClassNamesCapabilityBeforeSelection(t *testing.T) {
	for _, version := range []string{"2.1.272", ReferenceClient, "99.0.0", ""} {
		t.Run(version, func(t *testing.T) {
			g, f := newContextFixture(t)
			g.ConfigureDelegations(t.TempDir())
			rq := messages(strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"public"}]}`))
			delete(rq.headers, "X-Claude-Code-Request-Class")
			rq.headers["User-Agent"] = "claude-cli/" + version
			rq.headers["X-Claude-Code-Session-Id"] = "public-session"
			rq.headers["X-Claude-Code-Agent-Id"] = "unregistered-child"
			response := do(t, g, rq)
			body := bodyText(t, response)
			if response.StatusCode != 400 || !strings.Contains(body, "CONTEXT_REQUEST_CLASS_UNVERIFIED") || !strings.Contains(body, "X-Claude-Code-Request-Class") || !strings.Contains(body, "Update Claude Code") || f.Calls() != 0 || f.counts.Load() != 0 {
				t.Fatalf("capability refusal lost: status=%d body=%s", response.StatusCode, body)
			}
			if d := status(t, g); d.Agents.Unregistered != 0 || d.Requests.RefusedBy["CONTEXT_REQUEST_CLASS_UNVERIFIED"] != 1 || !d.Client.RequestClassRequired || d.Client.RequestClassMissing != 1 {
				t.Fatal("missing capability reached child selection")
			}
		})
	}
}

func TestRequestClassCapabilityIsIndependentOfVersionMatch(t *testing.T) {
	g, f := newContextFixture(t)
	for _, class := range []string{"", "main"} {
		rq := messages(strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"public"}]}`))
		rq.headers["X-Claude-Code-Session-Id"] = "public-session"
		rq.headers["X-Claude-Code-Request-Class"] = class
		rq.headers["User-Agent"] = "claude-cli/" + ReferenceClient
		response := do(t, g, rq)
		bodyText(t, response)
		if class == "" && response.StatusCode != 400 || class == "main" && response.StatusCode != 200 {
			t.Fatal("capability admission changed")
		}
	}
	if f.Calls() != 1 {
		t.Fatal("missing capability reached backend")
	}
	if d := status(t, g); !d.Client.Verified || d.Client.VerifiedMeaning != "version_match_only" || !d.Client.RequestClassRequired || d.Client.RequestClassMissing != 1 {
		t.Fatal("version match concealed the missing capability")
	}
	// An old version string carrying the required capability remains admissible.
	g, f = newContextFixture(t)
	rq := messages(strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"public"}]}`))
	rq.headers["X-Claude-Code-Session-Id"] = "public-session"
	rq.headers["User-Agent"] = "claude-cli/2.1.272"
	response := do(t, g, rq)
	bodyText(t, response)
	if d := status(t, g); response.StatusCode != 200 || f.Calls() != 1 || d.Client.Version != "2.1.272" || d.Client.Verified || d.Client.RequestClassMissing != 0 {
		t.Fatal("version string replaced capability check")
	}
}

func TestRequestClassRequirementDoesNotApplyToReadinessOrDisabledPolicy(t *testing.T) {
	g, _ := newContextFixture(t)
	response := do(t, g, request{method: "HEAD", path: "/api/hello", noAuth: true})
	bodyText(t, response)
	if response.StatusCode != 204 || status(t, g).Client.RequestClassMissing != 0 {
		t.Fatal("readiness acquired a message capability requirement")
	}
	g = start(t)
	if d := status(t, g); d.Client.RequestClassRequired || d.Client.RequestClassMissing != 0 {
		t.Fatal("disabled policy reported a capability requirement")
	}
}
