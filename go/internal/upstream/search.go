package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
)

// The client's search side query is answered from the backend's own standalone search
// endpoint: one plain JSON POST, no stream, no model turn. Same host and same credential as
// an inference request, so nothing new is trusted and no second secret exists.

// searchPath replaces the inference path. Derived rather than written out so the two can
// never drift onto different hosts, and checked so a change to Endpoint that stops matching
// is a refusal rather than a request to whatever the unreplaced string happens to name.
const (
	responsesSuffix = "/responses"
	searchSuffix    = "/alpha/search"
)

// searchTimeout bounds one search. The baseline's: min(overall, 45s). A side query the user
// is waiting on is not the place to spend ten minutes.
const searchTimeout = 45 * time.Second

type searchCounters struct{ requests, attempts, retries atomic.Int64 }
type SearchStats struct {
	Requests int64 `json:"requests"`
	Attempts int64 `json:"attempts"`
	Retries  int64 `json:"retries"`
}

func (d *Direct) SearchStats() SearchStats {
	return SearchStats{d.searchCounts.requests.Load(), d.searchCounts.attempts.Load(), d.searchCounts.retries.Load()}
}

// Refusals particular to the search path.
var (
	// ErrSearchUnavailable means the endpoint is not there. An alpha endpoint that is gone
	// is a different problem from one that is briefly unwell: the first ends the feature,
	// the second is worth one more try.
	ErrSearchUnavailable = errors.New("SEARCH_UNAVAILABLE")
	// ErrSearchEndpoint means the configured endpoint does not name a path this can derive
	// a search address from. Refused rather than guessed.
	ErrSearchEndpoint = errors.New("INVALID_ENDPOINT")
)

func (d *Direct) searchTarget() (string, error) {
	target := d.target()
	if !strings.HasSuffix(target, responsesSuffix) {
		return "", ErrSearchEndpoint
	}
	return strings.TrimSuffix(target, responsesSuffix) + searchSuffix, nil
}

// Search answers one side query.
//
// Retried exactly once, and only on a failure that could pass. A search is an idempotent
// read so one retry cannot duplicate an effect, and one is the limit because a side query
// the client is waiting on is not the place to spend a retry budget. The ledger is not
// consulted for ordinary sessions: search is not an inference. Verification
// requires explicit search opt-in and reserves each attempt before credentials.
func (d *Direct) Search(ctx context.Context, body []byte) ([]byte, error) {
	d.searchCounts.requests.Add(1)
	if d.Ledger != nil {
		if err := d.Ledger.verificationSearch(false); err != nil {
			return nil, err
		}
	}
	target, err := d.searchTarget()
	if err != nil {
		return nil, err
	}
	if err := d.deferred(time.Now()); err != nil {
		return nil, err
	}
	if d.Credentials == nil {
		return nil, &auth.Error{Category: auth.CategoryUnavailable}
	}
	credential, err := d.Credentials.Credential()
	if err != nil {
		return nil, err
	}
	if credential.Synthetic {
		return nil, ErrSyntheticMixing
	}
	version, err := d.clientVersion()
	if err != nil {
		return nil, err
	}

	// The reference client puts a session identity in the body and overrides whatever the
	// caller put there. Injected rather than asked for: the session belongs to the
	// transport, which is the thing that has one, and the caller that builds the query has
	// no business inventing an identity for a connection it does not own.
	body = withSearchSession(body, d.searchSessionID())

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	raw, failure := d.searchOnce(ctx, target, body, credential, version)
	if failure == nil {
		return raw, nil
	}
	var retryable Failure
	if !errors.As(failure, &retryable) || retryable.Disposition != Retryable {
		return nil, failure
	}
	if d.Ledger != nil {
		if err := d.Ledger.verificationSearch(true); err != nil {
			return nil, err
		}
	}
	// A 401 is worth another try only on a different token: the same one gets the same
	// answer. The store is read again because the Codex CLI may have refreshed it since, and
	// the provider still holds the session to its account (#91).
	if retryable.Category == "UNAUTHENTICATED" {
		fresh, err := d.Credentials.Credential()
		if err != nil || fresh.Synthetic || fresh.Token() == credential.Token() {
			return nil, failure
		}
		credential = fresh
	}
	d.searchCounts.retries.Add(1)
	raw, failure = d.searchOnce(ctx, target, body, credential, version)
	if failure != nil {
		return nil, failure
	}
	return raw, nil
}

func (d *Direct) searchOnce(ctx context.Context, target string, body []byte,
	credential auth.Credential, version string) ([]byte, error) {

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// The reference client's own search identity. originator is codex_exec here and
	// codex_cli_rs on an inference request: they are different callers to the backend and
	// sending one under the other's name would be claiming to be something else.
	request.Header.Set("Authorization", "Bearer "+credential.Token())
	request.Header.Set("ChatGPT-Account-ID", credential.Account)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Originator", "codex_exec")
	request.Header.Set("User-Agent", "codex_exec/"+version+" (Windows; x86_64) xterm-256color (codex_exec; "+version+")")
	request.Header.Set("Content-Length", strconv.Itoa(len(body)))
	// The turn envelope the reference client sends. Generated for this process and
	// describing nothing local: no path, no repository, no workspace, and nothing copied
	// from the user's own codex installation.
	request.Header.Set("x-codex-turn-metadata", searchTurnMetadata(time.Now()))

	d.searchCounts.attempts.Add(1)
	response, err := d.client().Do(request)
	if err != nil {
		if errors.Is(err, ErrRedirected) {
			return nil, ErrRedirected
		}
		return nil, ClassifyTransport(err)
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return nil, Failure{Category: "UNAUTHENTICATED", Disposition: Retryable}
	case response.StatusCode == http.StatusNotFound, response.StatusCode == http.StatusGone:
		return nil, ErrSearchUnavailable
	case response.StatusCode != http.StatusOK:
		failure := Failure{Category: "SEARCH_HTTP_ERROR", Status: response.StatusCode}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			failure = withRetryAfter(failure, response.Header, time.Now())
			d.deferUntil(failure)
		}
		return nil, failure
	}

	// Bounded by the same ceiling a streamed response gets. A search answer is web content
	// and the size of it is not this build's to trust.
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxSearchResponseBytes+1))
	if err != nil {
		return nil, ClassifyTransport(err)
	}
	if len(raw) > maxSearchResponseBytes {
		return nil, Failure{Category: "RESPONSE_TOO_LARGE", Disposition: Terminal}
	}
	if !json.Valid(raw) {
		return nil, Failure{Category: "SEARCH_RESPONSE_SHAPE", Disposition: Terminal}
	}
	return raw, nil
}

// maxSearchResponseBytes matches the streamed ceiling: 16 MiB.
const maxSearchResponseBytes = 16 * 1024 * 1024

// searchSessionID is one identity per transport, matching the reference client: a session
// is a session, and minting a fresh one per query would describe every search as the first.
func (d *Direct) searchSessionID() string {
	d.sessionOnce.Do(func() { d.session = randomUUID() })
	return d.session
}

// withSearchSession puts the identity at the front of the body.
//
// By prefix rather than by re-encoding. Round-tripping the JSON would reorder every key,
// and this is an alpha endpoint that has already answered 400 once for a body it did not
// recognise -- there is no reason to hand it a shape that differs from the reference
// client's in any way this build can avoid.
func withSearchSession(body []byte, session string) []byte {
	if len(body) == 0 || body[0] != '{' {
		return body
	}
	prefix := `{"id":"` + session + `",`
	if len(body) == 2 { // "{}"
		prefix = `{"id":"` + session + `"`
	}
	out := make([]byte, 0, len(prefix)+len(body)-1)
	out = append(out, prefix...)
	return append(out, body[1:]...)
}

// searchTurnMetadata is the codex-shaped turn envelope the search endpoint expects.
//
// The field set and its order are the reference client's, read from the Node baseline's
// searchEnvelope. Every identifier is generated here for this call; nothing describes this
// machine, this repository or this user.
func searchTurnMetadata(now time.Time) string {
	session, turn := randomUUID(), randomUUID()
	envelope := struct {
		InstallationID  string `json:"installation_id"`
		SessionID       string `json:"session_id"`
		ThreadID        string `json:"thread_id"`
		AgentName       string `json:"agent_name"`
		TurnID          string `json:"turn_id"`
		RootTurnID      string `json:"root_turn_id"`
		WindowID        string `json:"window_id"`
		WindowNumber    int    `json:"window_number"`
		ContextWindowID string `json:"context_window_id"`
		RequestKind     string `json:"request_kind"`
		ThreadSource    string `json:"thread_source"`
		Sandbox         string `json:"sandbox"`
		SandboxMode     string `json:"sandbox_mode"`
		AutoReview      bool   `json:"auto_review_enabled"`
		ReplAutoReview  bool   `json:"node_repl_auto_review_required"`
		ReplDisabled    bool   `json:"node_repl_disabled"`
		StartedAt       int64  `json:"turn_started_at_unix_ms"`
	}{
		InstallationID: randomUUID(), SessionID: session, ThreadID: session,
		AgentName: "/root", TurnID: turn, RootTurnID: turn,
		WindowID: session + ":0", WindowNumber: 0, ContextWindowID: randomUUID(),
		RequestKind: "turn", ThreadSource: "user", Sandbox: "none", SandboxMode: "read-only",
		AutoReview: false, ReplAutoReview: false, ReplDisabled: true,
		StartedAt: now.UnixMilli(),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// randomUUID is a version 4 UUID in the dashed form the reference client sends.
func randomUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(raw)
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32]
}
