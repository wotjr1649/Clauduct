package upstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
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
)

// Timeouts are per phase. One overall deadline would cut a long thinking response at the
// same moment it would cut a hung connection, and those are different failures.
const (
	connectTimeout = 30 * time.Second
	headerTimeout  = 120 * time.Second
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
	// ClientVersion is what the Codex CLI reports. It identifies this client to the
	// backend exactly as the reference client does.
	ClientVersion string
	// Route is the model and effort each request declares, checked against the budget.
	Model  string
	Effort string
	// Client is the HTTP client. Zero uses the shared one, which verifies certificates and
	// refuses redirects.
	Client *http.Client

	// endpoint overrides Endpoint. Unexported and set only by this package's tests: the
	// destination stays unreachable from configuration, which is the point of the constant.
	endpoint string
}

func (d *Direct) target() string {
	if d.endpoint != "" {
		return d.endpoint
	}
	return Endpoint
}

// NewDirect builds a transport with a client that cannot be talked out of verifying a
// certificate or following a redirect.
func NewDirect(credentials *auth.Provider, ledger *Ledger, version, model, effort string) *Direct {
	return &Direct{
		Credentials:   credentials,
		Ledger:        ledger,
		ClientVersion: version,
		Model:         model,
		Effort:        effort,
		Client:        newClient(),
	}
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
func (d *Direct) Execute(ctx context.Context, body []byte) (*Response, error) {
	if d.Ledger == nil {
		return nil, ErrBudgetExhausted
	}
	if err := d.Ledger.Reserve(d.Model, d.Effort, false); err != nil {
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

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.target(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyHeaders(request, credential, d.ClientVersion, len(body))

	response, err := d.client().Do(request)
	if err != nil {
		if errors.Is(err, ErrRedirected) {
			return nil, ErrRedirected
		}
		return nil, ClassifyTransport(err)
	}
	if response.StatusCode != http.StatusOK {
		failure := ClassifyStatus(response.StatusCode, response.Header, time.Now())
		response.Body.Close()
		return nil, failure
	}
	return &Response{Body: response.Body}, nil
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
