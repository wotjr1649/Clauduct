package providerprobe

import (
	"net/http"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3OptionalBlockEvidence records only enough metadata to join a blocked main request to its
// native run and observation window. The response writer must already have written a 503.
type p3OptionalBlockEvidence struct {
	Sequence        int       `json:"sequence"`
	ConnectionID    int64     `json:"connectionId"`
	OwnerPID        uint32    `json:"ownerPid"`
	OwnerCreated    time.Time `json:"ownerCreated"`
	TurnCanary      string    `json:"turnCanary"`
	Protocol        string    `json:"protocol"`
	Status          int       `json:"status"`
	MainRoot        bool      `json:"mainRoot"`
	ConnectionKnown bool      `json:"connectionKnown"`
}

const (
	p3OptionalAnthropicProtocol = "anthropic_messages"
	p3OptionalBedrockProtocol   = "bedrock_invoke"
)

func p3OptionalControlCase(c p3Case, configuration string) bool {
	if !c.Optional || configuration != p3Unprotected {
		return false
	}
	switch c.ID {
	case "R13a", "R13b", "R13c", "R13d", "R14n":
		return true
	default:
		return false
	}
}

// p3OptionalBlock is called only after the catch-all responder actually writes 503. Seen is
// Observer's decoded-body result; request contents, auth values, paths, and model IDs stay in memory.
func p3OptionalBlock(c p3Case, r *http.Request, seen providercert.Seen) *p3OptionalBlockEvidence {
	if r == nil || r.URL == nil || !p3OptionalControlCase(c, p3Unprotected) || seen.Method != http.MethodPost || r.Method != http.MethodPost ||
		seen.PathClass != "inference" || !seen.Inference || seen.Undecodable || !p3OptionalMainRoot(r) ||
		!p3OptionalHasCanary(seen.Canaries, "turn1") || seen.Sequence <= 0 {
		return nil
	}
	protocol, ok := p3OptionalProtocol(c, r.URL.Path)
	if !ok || !p3OptionalTemplateMatches(protocol, seen.PathTemplate) {
		return nil
	}
	connection := seen.Connection
	if connection == nil || connection.ID <= 0 || connection.Accepted.IsZero() || connection.Client == "" || connection.Server == "" ||
		connection.Owner.PID == 0 || connection.Owner.Created.IsZero() {
		return nil
	}
	return &p3OptionalBlockEvidence{
		Sequence:        seen.Sequence,
		ConnectionID:    connection.ID,
		OwnerPID:        connection.Owner.PID,
		OwnerCreated:    connection.Owner.Created,
		TurnCanary:      "turn1",
		Protocol:        protocol,
		Status:          http.StatusServiceUnavailable,
		MainRoot:        true,
		ConnectionKnown: true,
	}
}

func p3OptionalProtocol(c p3Case, path string) (string, bool) {
	if path == "/v1/messages" {
		return p3OptionalAnthropicProtocol, true
	}
	if c.ID == "R13b" && p3OptionalControlCase(c, p3Unprotected) && p3BedrockInvokePath(path) {
		return p3OptionalBedrockProtocol, true
	}
	return "", false
}

func p3BedrockInvokePath(path string) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) == 3 && parts[0] == "model" && parts[1] != "" && parts[1] != "." && parts[1] != ".." &&
		(parts[2] == "invoke" || parts[2] == "invoke-with-response-stream")
}

func p3OptionalTemplateMatches(protocol, pathTemplate string) bool {
	if !strings.HasPrefix(pathTemplate, "POST ") {
		return false
	}
	switch protocol {
	case p3OptionalAnthropicProtocol:
		return strings.HasSuffix(pathTemplate, "/v1/messages")
	case p3OptionalBedrockProtocol:
		return strings.HasSuffix(pathTemplate, "/{seg}/{seg}/{seg}")
	default:
		return false
	}
}

func p3OptionalMainRoot(r *http.Request) bool {
	classes := r.Header.Values("X-Claude-Code-Request-Class")
	if len(classes) != 1 || classes[0] != "main" {
		return false
	}
	for _, name := range []string{"X-Claude-Code-Agent-Id", "X-Claude-Code-Parent-Agent-Id"} {
		for _, value := range r.Header.Values(name) {
			if strings.TrimSpace(value) != "" {
				return false
			}
		}
	}
	return true
}

func p3OptionalHasCanary(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// p3OptionalControlProved accepts only the five optional controls' own blocked main request
// and one clean native run. An incomplete window is acceptable only when that native exited
// naturally and cleanly; deadline, drain, spawn, identity, and error failures remain failures.
func p3OptionalControlProved(c p3Case, obs *p3Observed) bool {
	if obs == nil || !p3OptionalControlCase(c, obs.Configuration) || obs.OptionalControlBlock == nil ||
		obs.NativeStarts != 1 || len(obs.Runs) != 1 || obs.SpawnFailed || obs.TimedOut || obs.CleanupFailed || obs.HeldProcess || obs.JobUnconfirmed ||
		obs.UnexpectedRunError || obs.IdentityError || obs.ReceiverFault || !obs.ReceiversDrained || obs.Dropped != 0 {
		return false
	}
	for _, record := range obs.CatchAll {
		if record.Undecodable {
			return false
		}
	}
	block := obs.OptionalControlBlock
	if block.Sequence <= 0 || block.ConnectionID <= 0 || block.TurnCanary != "turn1" || block.Status != http.StatusServiceUnavailable ||
		!block.MainRoot || !block.ConnectionKnown || !p3OptionalBlockSeen(obs.CatchAll, block) {
		return false
	}
	if block.Protocol != p3OptionalAnthropicProtocol && !(c.ID == "R13b" && block.Protocol == p3OptionalBedrockProtocol) {
		return false
	}
	// The requester is the case's native Job tree, not one generation (r5 scopeLedger): the run's
	// PID and creation are evidence only. The block still needs a known owner on its connection
	// (p3OptionalBlock, p3OptionalBlockSeen): a deliberately conservative join of the 503 to one
	// accepted connection, kept although an unknown owner leaves a real block INCONCLUSIVE.
	run := obs.Runs[0]
	if run.Index <= 0 || run.Gateway <= 0 || !run.NativeStarted || run.Exit == app.ExitCodeUnknown || run.CleanupFailed || run.UnexpectedError {
		return false
	}
	if p3OptionalWindowCompleted(run, block) {
		return true
	}
	return run.NaturalExit
}

func p3OptionalBlockSeen(seen []providercert.Seen, block *p3OptionalBlockEvidence) bool {
	matches := 0
	for _, record := range seen {
		connection := record.Connection
		if record.Sequence != block.Sequence || connection == nil || connection.ID != block.ConnectionID {
			continue
		}
		if record.Method != http.MethodPost || record.PathClass != "inference" || !record.Inference || record.Undecodable || !p3OptionalHasCanary(record.Canaries, block.TurnCanary) ||
			!p3OptionalTemplateMatches(block.Protocol, record.PathTemplate) ||
			connection.ID <= 0 || connection.Accepted.IsZero() || connection.Client == "" || connection.Server == "" ||
			connection.Owner.PID == 0 || connection.Owner.Created.IsZero() || connection.Owner.PID != block.OwnerPID || !connection.Owner.Created.Equal(block.OwnerCreated) {
			return false
		}
		matches++
	}
	return matches == 1
}

func p3OptionalWindowCompleted(run p3RunEvidence, block *p3OptionalBlockEvidence) bool {
	window := run.Window
	if window == nil || window.Kind != "OPTIONAL_BLOCKED_CONTROL" || window.GraceMs != 0 || !window.Completed ||
		window.DurationMs != p3ObservationWindowDuration.Milliseconds() || window.ElapsedMs < window.DurationMs {
		return false
	}
	trigger := window.Trigger
	// The blocked request went to CATCH_ALL directly, so its trigger has no product-gateway id.
	return trigger.Run == run.Index && trigger.Gateway == 0 && trigger.Seq == int64(block.Sequence) &&
		trigger.Connection == block.ConnectionID && trigger.Canary == block.TurnCanary && trigger.Category == block.Protocol && trigger.Status == block.Status
}
