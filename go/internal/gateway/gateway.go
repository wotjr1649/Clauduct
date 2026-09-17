// Package gateway owns the ephemeral loopback listener the native client talks to.
//
// WP02 scope: bind, session token, request boundary, authentication, per-request
// cancellation and shutdown. The routes it recognises answer with a fixed category rather
// than content — there is still no upstream client anywhere in this module, so a model
// request is not merely absent, it has no code path to travel.
//
// The rules here were measured against the installed claude 2.1.272 rather than recalled:
// readiness arrives as HEAD /api/hello with no credential at all, inference arrives as
// POST /v1/messages?beta=true carrying exactly one Authorization: Bearer header, and a
// trivial prompt already produces a 119 KB body.
package gateway

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The largest request body accepted, matching the Node baseline's requestBytes. A measured
// "ping" is already 119 KB of system prompt and tool schemas, so the ceiling has to leave
// room for images and long histories while still being a ceiling.
const maxRequestBytes = 32 * 1024 * 1024

// How long a client may take to finish a body it has already begun. PROPOSED, matching the
// Node baseline's 300s. Without a ceiling a client that opens a request and stops writing
// holds its admission slot forever, and enough of those exhaust the cap without ever
// sending anything.
const requestBodyTimeout = 300 * time.Second

// writeStall is how long one write to the client may block before the session gives up on
// it.
//
// Not a limit on how long a response may take. It is reset before every batch, so a client
// that keeps reading never meets it however long the answer runs -- which is the design the
// handoff asks for and the reason a single global WriteTimeout is the wrong tool.
//
// What it bounds is a client that stopped reading. The socket buffer fills, the next write
// blocks, and without this it blocks forever while an upstream request keeps running.
//
// A var rather than a const so a test can shorten it. The mechanism is the same at three
// seconds as at thirty, and a suite that spends half a minute per case proving it is a
// suite people start skipping. Nothing outside a test assigns to it.
var writeStall = 30 * time.Second

// writeChunk is how much of the response goes out under one deadline.
//
// The Node baseline's number: native-delivery.mjs writes 16 KiB at a time and waits for
// backpressure on each piece. Without a split, a batch large enough to exceed the bound
// gets a live client cut off for the sender's pacing rather than its own.
const writeChunk = 16 * 1024

// Header names that may never appear. cookie and proxy-authorization carry credentials
// this gateway did not issue and has no use for; a client that sends one is either not the
// client we think it is or is being driven by something that is not.
var bannedCredentialHeaders = []string{"Cookie", "Proxy-Authorization"}

// Header names that mean the request did not come straight from the local child.
var bannedProxyHeaders = []string{"Origin", "Sec-Fetch-Site", "Forwarded"}

// Gateway is one session's listener. Two concurrent sessions share nothing: separate
// listener, separate port, separate token, separate request registry.
type Gateway struct {
	listener  net.Listener
	server    *http.Server
	token     string
	expected  string // the exact Host this session answers to
	requests  *registry
	agents    *agentRegistry
	transport upstream.Transport
	served    chan error
	ring      *ring
	betas     *betaLedger
	limits    *limitLedger
	events    *eventLedger

	received   atomic.Int64
	modelLists atomic.Int64
	// Counted rather than refused. A subagent whose registration has not arrived, or whose
	// role has no route, runs on what the client asked for -- and these are how often that
	// happened, so "routing quietly did nothing" is a number rather than a silence.
	unregisteredAgents atomic.Int64
	unroutedRoles      atomic.Int64
	refused            atomic.Int64
	// client is what the client called itself, taken from the first request that named a
	// version in the shape this build recognises. Stored once: a session has one client,
	// and a later value would mean something this account cannot explain.
	client atomic.Value

	closeOnce sync.Once
	closeErr  error
}

// Start binds 127.0.0.1 on a port the operating system chooses and begins serving.
//
// A nil transport means none is configured, and every inference request then fails with
// NO_UPSTREAM_TRANSPORT. That is deliberate: a build with nowhere to send a request must
// say so at the point of use rather than appear to work.
//
// The port comes from bind, never from a search. Probing for a free port and closing it
// before rebinding opens a window where another process takes it; asking for :0 and
// keeping the listener has no such window. The caller passes the resulting address to the
// child, so there is never a guess about which port is live.
func Start(transport upstream.Transport) (*Gateway, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		listener.Close()
		return nil, err
	}
	if transport == nil {
		transport = upstream.None{}
	}
	g := &Gateway{
		listener:  listener,
		token:     token,
		expected:  listener.Addr().String(),
		requests:  newRegistry(),
		agents:    newAgentRegistry(),
		transport: transport,
		served:    make(chan error, 1),
		ring:      newRing(),
		betas:     newBetaLedger(),
		limits:    newLimitLedger(),
		events:    newEventLedger(),
	}
	g.server = &http.Server{
		Handler: http.HandlerFunc(g.handle),
		// Header deadline only. A global WriteTimeout would eventually cut a long
		// streaming response, and this server is going to carry exactly that. The
		// per-phase deadlines that replace it belong with the streaming code.
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
	}
	go func() {
		err := g.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		g.served <- err
	}()
	return g, nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Addr is the live bound address, host:port.
func (g *Gateway) Addr() string { return g.listener.Addr().String() }

// BaseURL is what the child's ANTHROPIC_BASE_URL is set to.
func (g *Gateway) BaseURL() string { return "http://" + g.Addr() }

// Token is the session bearer token. It is never logged, never placed on a command line,
// and never written to an error string.
func (g *Gateway) Token() string { return g.token }

// ReferenceClient is the client version this gateway's rules were measured against.
//
// Evidence, not a pin. Nothing here refuses a version and nothing should: the client
// updates itself, so a build that only runs against one version would be broken by design
// rather than by a defect. What the account gets instead is the observed version beside
// this one, so a session that starts failing after an update says so in one line rather
// than becoming a bisect.
const ReferenceClient = "2.1.274"

// clientAgent matches the client naming itself.
//
// Both product names and a trailing suffix, because that is what was measured rather than
// what was assumed: /v1/models arrives as claude-code/2.1.274 and /v1/messages as
// claude-cli/2.1.274 (external, sdk-cli). The first version of this pattern anchored the
// end and required claude-cli, so it matched nothing at all and the account reported an
// empty version -- found by reading a real session's account, which is the thing this
// field exists to make possible.
//
// The readiness probe arrives as Bun/1.4.3 and is not recorded: this account is a file that
// outlives the session, so it holds a version number or nothing.
var clientAgent = regexp.MustCompile(`^claude-(?:cli|code)/([0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z.+-]*)`)

// noteClient records the client's version the first time it names one.
func (g *Gateway) noteClient(agent string) {
	if g.client.Load() != nil {
		return
	}
	match := clientAgent.FindStringSubmatch(agent)
	if match == nil {
		return
	}
	g.client.CompareAndSwap(nil, match[1])
}

// ClientVersion is the version the client named, or empty if it never named one.
func (g *Gateway) ClientVersion() string {
	version, _ := g.client.Load().(string)
	return version
}

// Stats reports counts only. Nothing here is derived from request content.
// ModelLists reports how many times the client asked for the model list.
//
// Counted separately from the rest because it answers a question nothing else can: whether
// the client has gateway discovery on at all. Without it the user's picker is the built-in
// Anthropic one, and the only difference visible from here is that this request never
// arrives.
func (g *Gateway) ModelLists() int64 { return g.modelLists.Load() }

// Unrouted reports the subagent requests that kept the client's own model: one count for a
// registration that never arrived, one for a role with no route.
func (g *Gateway) Unrouted() (unregistered, unrouted int64) {
	return g.unregisteredAgents.Load(), g.unroutedRoles.Load()
}

func (g *Gateway) Stats() (received, refused, active int64) {
	return g.received.Load(), g.refused.Load(), int64(g.requests.count())
}

func (g *Gateway) handle(w http.ResponseWriter, r *http.Request) {
	g.received.Add(1)
	g.noteClient(r.Header.Get("User-Agent"))

	// The record opens before anything is checked, so a refused boundary, version or
	// encoding is a diagnosed failure rather than an unrecorded 400. The Node baseline
	// orders it this way on purpose and the reason survives the rewrite.
	//
	// Reading the diagnostics is not traffic. Sixteen status reads would otherwise erase
	// every record of what the session did, which is the one thing the reader came for.
	if r.URL.Path != statusPath {
		entry := g.ring.open(r.Method, r.URL.Path)
		defer entry.finish()
		w = &tracked{ResponseWriter: w, rec: entry}
	}

	if bad, ok := g.checkBoundary(r); !ok {
		g.refuse(w, bad)
		return
	}

	// Readiness answers without a credential because that is what the client sends: the
	// measured probe carries no Authorization at all. It must therefore reveal nothing —
	// no body, no token, no account, no upstream state. A credential is still validated
	// when one is offered, so a wrong token is never quietly accepted anywhere.
	if r.Method == http.MethodHead && r.URL.Path == "/api/hello" {
		if r.Header.Get("Authorization") != "" && !g.authorized(r) {
			g.refuse(w, refuseSession)
			return
		}
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !g.authorized(r) {
		g.refuse(w, refuseSession)
		return
	}

	if r.URL.Path == "/v1/messages" {
		g.handleMessages(w, r)
		return
	}

	if r.URL.Path == "/clauduct/agents" {
		g.handleAgents(w, r)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == statusPath {
		g.handleStatus(w)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		g.modelLists.Add(1)
		g.handleModels(w)
		return
	}

	// Everything else is genuinely not a route here. Saying so with a fixed code is the
	// point: a silent 200 would let a caller believe a route works, and forwarding an
	// unknown path upstream would make this an open relay.
	g.refuse(w, refuseRoute)
}

// checkBoundary establishes that this request came from the local child over the loopback
// address this session owns, and carries no credential this gateway did not issue.
func (g *Gateway) checkBoundary(r *http.Request) (refusal, bool) {
	// A repeated header name is how request smuggling hides a second value behind the one
	// a reader checks. Nothing the measured client sends repeats, so refusing costs
	// nothing real and removes the whole class.
	for _, values := range r.Header {
		if len(values) > 1 {
			return refuseHeader, false
		}
	}
	for name := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-forwarded-") {
			return refuseBoundary, false
		}
	}
	for _, name := range bannedProxyHeaders {
		if r.Header.Get(name) != "" {
			return refuseBoundary, false
		}
	}
	// Host is compared to the exact address this session bound, not to "something
	// loopback-ish". A request naming any other host reached this socket by a route the
	// child does not use, which is what DNS rebinding looks like from in here.
	if r.Host != g.expected {
		return refuseBoundary, false
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || !isLoopback(host) {
		return refuseBoundary, false
	}
	for _, name := range bannedCredentialHeaders {
		if r.Header.Get(name) != "" {
			return refuseCredential, false
		}
	}
	// x-api-key is the other place the Anthropic protocol carries a credential. The
	// measured client does not send it, so this is forward compatibility, not an observed
	// need: a future version may present the same session token in both headers and
	// discovery must not break over that. A value that is not this session's token is a
	// foreign credential and is refused — it is never compared loosely and never
	// forwarded anywhere.
	if key := r.Header.Get("X-Api-Key"); key != "" && !g.tokenEquals(key) {
		return refuseCredential, false
	}
	return refusal{}, true
}

func isLoopback(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// tokenEquals compares in constant time. A length-dependent early exit would let a caller
// on this machine learn the token one byte at a time.
func (g *Gateway) tokenEquals(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(g.token)) == 1
}

func (g *Gateway) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	rest, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return false
	}
	return g.tokenEquals(rest)
}

func isJSON(contentType string) bool {
	if contentType == "" {
		return false
	}
	media, _, err := mime.ParseMediaType(contentType)
	return err == nil && media == "application/json"
}

// Close stops accepting, cancels the requests this gateway owns, waits for handlers up to
// the context deadline, and releases the listener.
//
// The order is the contract: refuse new work, then cancel what is in flight, then wait. A
// bare Shutdown waits for handlers to finish on their own, so one request blocked on a
// slow read would hold the whole exit open until the deadline expired.
//
// Idempotent, and it has to be: shutdown can be reached from a normal exit and from a
// cancellation path at the same time, and the serve result can only be received once. A
// second caller reading an already-drained channel would hang forever holding up the very
// cleanup it was trying to perform.
func (g *Gateway) Close(ctx context.Context) error {
	g.closeOnce.Do(func() {
		g.requests.closeAll(errShuttingDown)
		shutdownErr := g.server.Shutdown(ctx)
		if shutdownErr != nil {
			// A handler that will not return must not keep the port bound. Force the
			// connections closed so the resource is released, and still report the
			// graceful failure rather than replacing it with a success.
			g.server.Close()
		}
		serveErr := <-g.served
		if shutdownErr != nil {
			g.closeErr = shutdownErr
			return
		}
		g.closeErr = serveErr
	})
	return g.closeErr
}
