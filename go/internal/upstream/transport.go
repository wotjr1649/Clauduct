package upstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
)

// Endpoint is where an inference request goes. It is a constant of this build, not
// something a request, a project file or an environment variable can move: a credential is
// attached to every request, and letting anything else choose the destination is the way a
// credential ends up somewhere it was never meant to go.
const Endpoint = "https://chatgpt.com/backend-api/codex/responses"

// Refusals this transport produces on its own.
var (
	// ErrSyntheticMixing means a real credential met a fixture transport or the reverse.
	// Both directions are a defect: one sends a real credential to a test, the other tests
	// nothing about the real path.
	ErrSyntheticMixing = errors.New("SYNTHETIC_CREDENTIAL_MIXING")
	// ErrRedirected means the backend tried to move the request elsewhere. A redirect
	// carrying a credential to a new origin is the thing this refuses.
	ErrRedirected = errors.New("UPSTREAM_REDIRECT_REFUSED")
	// ErrNoClientVersion means nothing can say what this client is. Every request
	// identifies itself, and inventing a version would be claiming to be something else.
	ErrNoClientVersion = errors.New("CLI_VERSION_UNREADABLE")
)

// Timeouts are per phase. One overall deadline would cut a long thinking response at the
// same moment it would cut a hung connection, and those are different failures.
const (
	connectTimeout = 30 * time.Second
	headerTimeout  = 120 * time.Second
	// idleTimeout ends a response the backend has stopped sending: no byte for this long
	// after the headers. It is the Node baseline's rule -- req.setTimeout(600000), which
	// fires on socket inactivity and reported UPSTREAM_IDLE_TIMEOUT -- and it never cuts an
	// answer that is still arriving, however long that takes.
	idleTimeout = 10 * time.Minute
	// overallTimeout bounds one backend request from dial to last byte.
	//
	// The phase deadlines above are the better instrument and stay: a single overall clock
	// cannot tell a backend that never answered from one answering slowly, and cutting a
	// long correct answer is a worse failure than waiting for it. But they leave one case
	// open. A backend that keeps sending -- a keepalive every few seconds, a token a
	// minute -- meets no phase deadline and never ends, and a request that never ends holds
	// a goroutine, a connection and the user's subscription for as long as it likes.
	//
	// An hour is a ceiling for that case, not a budget for an answer. The baseline had no
	// overall cap: its ten minutes was the idle rule above, misread here once as an overall
	// one, which would lose an answer still streaming at ten minutes (#82). The longest of
	// 400 measured requests (2026-09-16 to 23) took 218 seconds.
	overallTimeout = 60 * time.Minute
)

// Direct is the real transport.
//
// It is the only place in this module that reaches a network, and it cannot be constructed
// without a ledger: a request that is not first reserved against a budget is not sent.
type Direct struct {
	// Credentials supplies the credential, read at the moment a request is sent rather
	// than at startup.
	Credentials *auth.Provider
	// Ledger authorises and records each attempt.
	Ledger *Ledger
	// The search session identity, one per transport and minted on first use.
	sessionOnce sync.Once
	session     string
	// Version reports what the installed Codex CLI calls itself. It is a function and not
	// a string because resolving it runs a subprocess, and a session that only ran
	// --version must not spawn one. Nothing is read until a request is actually sent.
	Version func() (string, error)
	// Client is the HTTP client. Zero uses the shared one, which verifies certificates and
	// refuses redirects.
	Client *http.Client

	// endpoint overrides Endpoint. Unexported and set only by this package's tests: the
	// destination stays unreachable from configuration, which is the point of the constant.
	endpoint string
	// overallFor and idleFor replace the product bounds in a test. Zero is the product.
	overallFor   time.Duration
	idleFor      time.Duration
	counts       countConnections
	searchCounts searchCounters
	// notBefore is when the backend last said this account may come back, in Unix
	// nanoseconds. Every attempt waits for it, not only the one that was told (#91).
	notBefore atomic.Int64
	// sent is the version resolved for a request, once one has been.
	sent atomic.Pointer[string]
}

// clientVersion resolves the version every request identifies itself with and remembers
// it, so the session account can say which Codex its requests claimed to be.
func (d *Direct) clientVersion() (string, error) {
	if d.Version == nil {
		return "", ErrNoClientVersion
	}
	version, err := d.Version()
	if err == nil {
		d.sent.Store(&version)
	}
	return version, err
}

// SentVersion is the Codex CLI version resolved for this transport's requests, or empty
// before the first. It never resolves one itself: a session that sent nothing must not have
// spawned a subprocess.
func (d *Direct) SentVersion() string {
	if version := d.sent.Load(); version != nil {
		return *version
	}
	return ""
}

// RetryDeferred is an attempt refused here, before a credential or a socket, because the
// time the backend named has not come. Answered as the backend's own 429 would be.
const RetryDeferred = "UPSTREAM_RETRY_DEFERRED"

func (d *Direct) deferred(now time.Time) error {
	until := time.Unix(0, d.notBefore.Load())
	if !now.Before(until) {
		return nil
	}
	return Failure{Category: RetryDeferred, Disposition: Deferred, RetryAfter: until.Sub(now), RetryAt: until}
}

func (d *Direct) deferUntil(failure Failure) {
	if failure.Disposition != Deferred {
		return
	}
	for at := failure.RetryAt.UnixNano(); ; {
		current := d.notBefore.Load()
		if at <= current || d.notBefore.CompareAndSwap(current, at) {
			return
		}
	}
}

func (d *Direct) target() string {
	if d.endpoint != "" {
		return d.endpoint
	}
	return Endpoint
}

// NewDirect builds a transport with a client that cannot be talked out of verifying a
// certificate or following a redirect.
func NewDirect(credentials *auth.Provider, ledger *Ledger, version func() (string, error)) *Direct {
	return &Direct{
		Credentials: credentials,
		Ledger:      ledger,
		Version:     version,
		Client:      newClient(),
	}
}

// Fixed returns a version function for a value already in hand.
func Fixed(version string) func() (string, error) {
	return func() (string, error) { return version, nil }
}

// One client for the process. Its configuration does not vary by caller, and sharing it
// keeps connection pooling rather than dialling afresh for every request.
var sharedClient = newClient()

func (d *Direct) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return sharedClient
}

func newClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// The user's proxy and CA configuration is their own network and is left alone. What
	// is not left alone is verification: InsecureSkipVerify is never set, whatever the
	// network looks like, and MinVersion is pinned so a downgrade is not available either.
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSHandshakeTimeout = connectTimeout
	transport.ResponseHeaderTimeout = headerTimeout
	// Compression is not requested, so there is no decompressed size to bound and no
	// decompression bomb to defend against. The baseline asks for identity for the same
	// reason.
	transport.DisableCompression = true

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Following it would carry the credential to whatever origin the response
			// named, which is the one thing an attacker controlling a response would ask
			// for.
			return ErrRedirected
		},
	}
}

// Execute sends one inference request.
//
// The order matters. The budget is consulted first, before a credential is read and before
// anything is dialled: a request that is not authorised must not cost a credential read,
// let alone a socket. Nothing in this function retries — see MaxGatewayRetries for the
// measurement behind that.
func (d *Direct) Execute(ctx context.Context, call Call) (*Response, error) {
	if d.Ledger == nil {
		return nil, ErrBudgetExhausted
	}
	// Authorised against what is actually in the body, not only against what the caller
	// says the route is. The budget exists to stop a request running on a model nobody
	// approved, and a check that trusts a claim beside the payload rather than the payload
	// would be satisfied by the one mistake it is there to catch.
	if err := agreesWithBody(call); err != nil {
		return nil, err
	}
	if err := d.deferred(time.Now()); err != nil {
		return nil, err
	}
	if err := d.Ledger.Reserve(Attempt{
		Requested: call.Requested,
		Model:     call.Model,
		Effort:    call.Effort,
		Source:    call.Source,
	}); err != nil {
		return nil, err
	}

	if d.Credentials == nil {
		return nil, &auth.Error{Category: auth.CategoryUnavailable}
	}
	credential, err := d.Credentials.Credential()
	if err != nil {
		return nil, err
	}
	// A synthetic credential must not reach the network, and a real one must not be used
	// to exercise a fixture. Both directions are checked because both are defects.
	if credential.Synthetic {
		return nil, ErrSyntheticMixing
	}

	version, err := d.clientVersion()
	if err != nil {
		return nil, err
	}

	// The deadline covers the body too, so it cannot be released when this returns. It is
	// released when the caller closes the body, which is the one event that means the
	// request is over however it ended.
	ctx, cancel := context.WithTimeout(ctx, d.overall())

	body := withSession(call.Body, call.Session)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.target(), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, err
	}
	applyHeaders(request, credential, version, len(body))
	if call.Session != "" {
		request.Header.Set("session-id", call.Session)
	}

	response, err := d.client().Do(request)
	if err != nil {
		cancel()
		if errors.Is(err, ErrRedirected) {
			return nil, ErrRedirected
		}
		return nil, ClassifyTransport(err)
	}
	if response.StatusCode != http.StatusOK {
		failure := ClassifyStatus(response.StatusCode, response.Header, time.Now())
		d.deferUntil(failure)
		if response.StatusCode == http.StatusBadRequest {
			raw, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
			if readErr == nil && len(raw) <= 64*1024 && codex.ContextLimit(raw) {
				failure.Category = "CONTEXT_LENGTH_EXCEEDED"
			}
		}
		response.Body.Close()
		cancel()
		return nil, failure
	}
	return &Response{
		Body:   &releaseOnClose{ReadCloser: response.Body, release: cancel, idle: time.AfterFunc(d.idle(), cancel), every: d.idle()},
		Header: response.Header,
	}, nil
}

// releaseOnClose lets go of the request's deadline when its body is closed, and ends the
// request when the backend goes quiet for longer than the idle bound.
//
// Without it the deadline leaks a timer and a goroutine for every request, and with a naive
// defer it would fire the moment Execute returns and cut every stream at its first byte.
type releaseOnClose struct {
	io.ReadCloser
	once    sync.Once
	release context.CancelFunc
	idle    *time.Timer
	every   time.Duration
}

// Read restarts the idle bound on every byte the backend sends.
func (r *releaseOnClose) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.idle.Reset(r.every)
	}
	return n, err
}

func (r *releaseOnClose) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() {
		r.idle.Stop()
		r.release()
	})
	return err
}

// overall and idle are the bounds on one request. Overridable so the properties can be
// tested in a second rather than in minutes; the product never sets them.
func (d *Direct) overall() time.Duration {
	if d.overallFor > 0 {
		return d.overallFor
	}
	return overallTimeout
}

func (d *Direct) idle() time.Duration {
	if d.idleFor > 0 {
		return d.idleFor
	}
	return idleTimeout
}

// applyHeaders builds the identity the reference client presents. It is one function so a
// probe and the gateway cannot drift apart on the wire.
func applyHeaders(request *http.Request, credential auth.Credential, version string, size int) {
	request.Header.Set("Authorization", "Bearer "+credential.Token())
	request.Header.Set("chatgpt-account-id", credential.Account)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	// No compression: nothing to decompress, nothing to bound, no bomb to defend against.
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Content-Length", strconv.Itoa(size))
	request.Header.Set("Version", version)
	request.Header.Set("User-Agent", "codex-cli/"+version+" (Windows; x64)")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("Openai-Beta", "responses=experimental")
}

// withSession adds the session's prompt_cache_key to an encoded request object. The body is
// the gateway's own json.Marshal output, so it ends in the object's closing brace; anything
// else goes unchanged rather than be rewritten on a guess.
func withSession(body []byte, session string) []byte {
	trimmed := bytes.TrimSpace(body)
	if session == "" || len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' {
		return body
	}
	key, _ := json.Marshal(session)
	out := append([]byte(nil), trimmed[:len(trimmed)-1]...)
	if len(bytes.TrimSpace(out)) > 1 {
		out = append(out, ',')
	}
	out = append(out, `"prompt_cache_key":`...)
	return append(append(out, key...), '}')
}

// ErrRouteMismatch means the declared route is not the one the body would run on.
var ErrRouteMismatch = errors.New("ROUTE_MISMATCH")

// agreesWithBody checks the declared route against the request that will be sent.
func agreesWithBody(call Call) error {
	var declared struct {
		Model     string `json:"model"`
		Reasoning *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	if err := json.Unmarshal(call.Body, &declared); err != nil {
		return ErrRouteMismatch
	}
	effort := ""
	if declared.Reasoning != nil {
		effort = declared.Reasoning.Effort
	}
	if declared.Model != call.Model || effort != call.Effort {
		return ErrRouteMismatch
	}
	return nil
}
