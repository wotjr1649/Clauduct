package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
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
// consulted: a search is not an inference and counting it as one would make a budget stated
// in inferences stop meaning that.
func (d *Direct) Search(ctx context.Context, body []byte) ([]byte, error) {
	target, err := d.searchTarget()
	if err != nil {
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
	if d.Version == nil {
		return nil, ErrNoClientVersion
	}
	version, err := d.Version()
	if err != nil {
		return nil, err
	}

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
			failure.Disposition = Retryable
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
