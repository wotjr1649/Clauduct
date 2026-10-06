package providercert

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Counter is what a destination saw at the TCP read, before TLS: for a wrong destination a
// handshake alone is already a non-zero count.
type Counter struct {
	Bytes atomic.Int64
	Conns atomic.Int64
}

type countingConn struct {
	net.Conn
	counter *Counter
}

func (c countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.counter.Bytes.Add(int64(n))
	return n, err
}

type countingListener struct {
	net.Listener
	counter *Counter
}

func (l countingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.counter.Conns.Add(1)
	return countingConn{conn, l.counter}, nil
}

// Canary is one turn's marker in the prompt. Only its digest goes into evidence.
type Canary struct {
	ID    string
	Value []byte
}

// NewCanary returns a fresh 128-bit hex canary.
func NewCanary(id string) (Canary, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return Canary{}, err
	}
	return Canary{ID: id, Value: []byte("clauduct-probe-" + hex.EncodeToString(b))}, nil
}

// Digest is what evidence records instead of the canary.
func (c Canary) Digest() string {
	sum := sha256.Sum256(c.Value)
	return hex.EncodeToString(sum[:])
}

// patterns are the forms the canary is searched in: raw, and standard and URL-safe base64 at
// each of the three byte alignments, cut to the characters the canary bytes alone determine.
func (c Canary) patterns() [][]byte {
	out := [][]byte{c.Value}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		for k := 0; k < 3; k++ {
			s := enc.EncodeToString(append(make([]byte, k), c.Value...))
			start, end := (k+2)/3*4, (k+len(c.Value))/3*4
			if end > start {
				out = append(out, []byte(s[start:end]))
			}
		}
	}
	return out
}

// maxScannedBody bounds how much of one request body is read looking for a canary.
const maxScannedBody = 64 << 20

// errUndecodable marks a body whose encoding the probe cannot undo.
var errUndecodable = errors.New("UNDECODABLE_EGRESS")

// scanner looks for canaries in a byte stream fed in pieces, keeping only a sliding window as
// long as the longest pattern. One scanner spans every message of a WebSocket connection, so a
// canary split across messages is still found.
type scanner struct {
	canaries []Canary
	patterns []scanPattern
	longest  int
	window   []byte
	found    map[string]bool
}

type scanPattern struct {
	id    string
	value []byte
}

func newScanner(canaries []Canary) *scanner {
	s := &scanner{found: map[string]bool{}}
	s.add(canaries)
	return s
}

// add extends the search to canaries not yet searched for, keeping the window: a socket opened
// in turn 1 must still be searched for turn 2's canary.
func (s *scanner) add(canaries []Canary) {
	for _, c := range canaries {
		if slices.ContainsFunc(s.canaries, func(have Canary) bool { return have.ID == c.ID }) {
			continue
		}
		s.canaries = append(s.canaries, c)
		for _, p := range c.patterns() {
			s.patterns = append(s.patterns, scanPattern{c.ID, p})
			s.longest = max(s.longest, len(p))
		}
	}
}

func (s *scanner) feed(b []byte) {
	s.window = append(s.window, b...)
	for _, p := range s.patterns {
		if !s.found[p.id] && bytes.Contains(s.window, p.value) {
			s.found[p.id] = true
		}
	}
	if keep := s.longest - 1; keep > 0 && len(s.window) > keep {
		s.window = append(s.window[:0], s.window[len(s.window)-keep:]...)
	}
}

func (s *scanner) ids() []string {
	ids := []string{}
	for _, c := range s.canaries {
		if s.found[c.ID] {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// scan reads at most maxScannedBody bytes of r and returns the IDs of canaries found. A body
// longer than that was not searched in full, which is undecodable rather than clean.
func scan(r io.Reader, canaries []Canary) ([]string, error) {
	s := newScanner(canaries)
	buf := make([]byte, 64<<10)
	var read int64
	for {
		n, err := r.Read(buf)
		read += int64(n)
		s.feed(buf[:n])
		if errors.Is(err, io.EOF) {
			return s.ids(), nil
		}
		if err != nil {
			return s.ids(), err
		}
		if read > maxScannedBody {
			return s.ids(), errUndecodable
		}
	}
}

// decoded undoes the request's Content-Encoding for gzip and deflate. Any other coding, br and
// zstd included, is undecodable: the standard library has no decoder and the verdict treats it
// as inconclusive rather than as clean.
func decoded(header http.Header, body io.Reader) (io.Reader, error) {
	var codings []string
	for _, value := range header.Values("Content-Encoding") {
		for _, part := range strings.Split(value, ",") {
			if part = strings.ToLower(strings.TrimSpace(part)); part != "" && part != "identity" {
				codings = append(codings, part)
			}
		}
	}
	// Codings are listed in the order applied, so they are undone from the last.
	for i := len(codings) - 1; i >= 0; i-- {
		var err error
		switch codings[i] {
		case "gzip", "x-gzip":
			body, err = gzip.NewReader(body)
		case "deflate":
			body, err = zlib.NewReader(body)
		default:
			return nil, errUndecodable
		}
		if err != nil {
			return nil, errUndecodable
		}
	}
	return body, nil
}

// Seen is one request a receiver observed: never its body or header values.
type Seen struct {
	At          time.Time `json:"at"`
	Host        string    `json:"host"`
	Method      string    `json:"method"`
	PathClass   string    `json:"pathClass"`
	Inference   bool      `json:"inference"`
	Account     bool      `json:"account"`
	WebSocket   bool      `json:"webSocket"`
	Undecodable bool      `json:"undecodable"`
	Canaries    []string  `json:"canaries,omitempty"`
	// Authority is a proxied tunnel's host:port. Kept out of evidence: it can name a loopback
	// port, and the probe only needs it in memory to attribute a tunnel to its own receivers.
	Authority string `json:"-"`
}

// maxSeen bounds one receiver's record; requests past it are still counted.
const maxSeen = 512

// Observer is the handler behind every probe receiver. It classifies and scans each request,
// then lets Respond answer it. Respond never sees the body: it has been consumed.
type Observer struct {
	ID       string
	Canaries func() []Canary
	Respond  func(w http.ResponseWriter, r *http.Request, seen Seen)

	mu      sync.Mutex
	seen    []Seen
	total   int
	dropped int
	// stop ends WebSocket reads, which outlive the HTTP server's Close once hijacked.
	stop     chan struct{}
	stopOnce sync.Once
}

func (o *Observer) stopped() chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stop == nil {
		o.stop = make(chan struct{})
	}
	return o.stop
}

func (o *Observer) shutdown() {
	ch := o.stopped()
	o.stopOnce.Do(func() { close(ch) })
}

// Seen returns a snapshot of the requests observed and how many were not kept.
func (o *Observer) Seen() (seen []Seen, total, dropped int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Seen(nil), o.seen...), o.total, o.dropped
}

func (o *Observer) ServeHTTP(w http.ResponseWriter, r *http.Request) { o.observe(w, r, r.Host) }

func (o *Observer) canaries() []Canary {
	if o.Canaries == nil {
		return nil
	}
	return o.Canaries()
}

func (o *Observer) observe(w http.ResponseWriter, r *http.Request, authority string) {
	seen := Seen{At: time.Now(), Host: hostOnly(authority), Method: r.Method, PathClass: PathClass(r.URL.Path),
		Inference: IsInference(r.URL.Path), Account: IsAccount(authority, r.URL.Path)}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		seen.WebSocket = true
		o.webSocket(w, r, o.record(seen))
		return
	}
	if body, err := decoded(r.Header, r.Body); err != nil {
		seen.Undecodable = true
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxScannedBody))
	} else if ids, err := scan(body, o.canaries()); err != nil {
		seen.Undecodable = true
	} else {
		seen.Canaries = ids
	}
	o.record(seen)
	if o.Respond != nil {
		o.Respond(w, r, seen)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// webSocket accepts the upgrade and scans every message for as long as the connection or the
// receiver lives. The record at index is updated as canaries appear, so a snapshot taken while
// the socket is still open already sees them. The library unmasks frames and undoes
// permessage-deflate; any end other than a normal close is undecodable.
func (o *Observer) webSocket(w http.ResponseWriter, r *http.Request, index int) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionContextTakeover})
	if err != nil {
		o.update(index, func(s *Seen) { s.Undecodable = true })
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxScannedBody)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-o.stopped():
			cancel()
		case <-ctx.Done():
		}
	}()
	stream := newScanner(o.canaries())
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status != websocket.StatusNormalClosure && status != websocket.StatusGoingAway {
				o.update(index, func(s *Seen) { s.Undecodable = true })
			}
			return
		}
		stream.add(o.canaries())
		stream.feed(data)
		ids := stream.ids()
		o.update(index, func(s *Seen) { s.Canaries = ids })
	}
}

// record keeps seen and returns its index, or -1 once the record is full. Requests past the
// bound are still counted in total and dropped, which the verdict reads as a receiver fault.
func (o *Observer) record(seen Seen) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.total++
	if len(o.seen) < maxSeen {
		o.seen = append(o.seen, seen)
		return len(o.seen) - 1
	}
	o.dropped++
	return -1
}

func (o *Observer) update(index int, fn func(*Seen)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if index >= 0 && index < len(o.seen) {
		fn(&o.seen[index])
	}
}

// Receiver is one loopback destination the probe owns.
type Receiver struct {
	URL      string
	Counter  *Counter
	Observer *Observer
	// CertSHA256 is the hex SHA-256 of a TLS receiver's certificate DER: the pin a synthetic
	// stored gateway credential carries.
	CertSHA256 string
	server     *http.Server
	done       chan error
}

// Close stops the receiver. Bytes that arrived before Close are already counted.
func (r *Receiver) Close() error {
	if r == nil || r.server == nil {
		return nil
	}
	if r.Observer != nil {
		r.Observer.shutdown()
	}
	err := r.server.Close()
	if serveErr := <-r.done; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	return err
}

// Listen starts a plain HTTP receiver on 127.0.0.1.
func Listen(o *Observer) (*Receiver, error) { return listen(o, nil, nil, o) }

// ListenUnix starts a plain HTTP receiver on an AF_UNIX socket at path, the destination a
// settings ANTHROPIC_UNIX_SOCKET would name. URL is the socket path.
func ListenUnix(o *Observer, path string) (*Receiver, error) {
	raw, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	counter := &Counter{}
	r := &Receiver{URL: path, Counter: counter, Observer: o, server: &http.Server{Handler: o, ReadHeaderTimeout: 10 * time.Second}, done: make(chan error, 1)}
	go func() { r.done <- r.server.Serve(countingListener{raw, counter}) }()
	return r, nil
}

// ListenTLS starts an HTTPS receiver on 127.0.0.1 with a fresh self-signed certificate.
func ListenTLS(o *Observer) (*Receiver, error) {
	cert, der, err := issue([]string{"127.0.0.1", "localhost"}, nil, nil)
	if err != nil {
		return nil, err
	}
	return listen(o, &cert, der, o)
}

func listen(handler http.Handler, cert *tls.Certificate, der []byte, o *Observer) (*Receiver, error) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	counter := &Counter{}
	var l net.Listener = countingListener{raw, counter}
	r := &Receiver{Counter: counter, Observer: o, server: &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}, done: make(chan error, 1)}
	scheme := "http"
	if cert != nil {
		scheme = "https"
		sum := sha256.Sum256(der)
		r.CertSHA256 = hex.EncodeToString(sum[:])
		l = tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{*cert}, NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12})
	}
	r.URL = scheme + "://" + raw.Addr().String()
	go func() { r.done <- r.server.Serve(l) }()
	return r, nil
}

// Request classes. Paths are matched, never the full URL.
var inferencePathParts = []string{"/v1/messages", "/v1/complete", "/invoke", ":rawpredict", ":streamrawpredict"}

// IsInference reports whether a path is a model request on any provider's wire format,
// count_tokens included: it carries the conversation too.
func IsInference(path string) bool {
	p := strings.ToLower(path)
	for _, part := range inferencePathParts {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

// PathClass is a path with its variable parts dropped, for evidence.
func PathClass(path string) string {
	p := strings.ToLower(path)
	switch {
	case IsInference(p):
		return "inference"
	case strings.HasPrefix(p, "/v1/models"):
		return "models"
	case strings.Contains(p, "/managed/settings") || strings.Contains(p, "/claude_code/settings"):
		return "managed-settings"
	case strings.Contains(p, "/oauth") || strings.Contains(p, "/auth/") || strings.Contains(p, "/login"):
		return "account"
	case strings.Contains(p, "/v1/traces") || strings.Contains(p, "/v1/metrics") || strings.Contains(p, "/v1/logs"):
		return "otlp"
	default:
		return "other"
	}
}

var accountHosts = []string{"claude.ai", "claude.com", "console.anthropic.com", "platform.claude.com"}
var accountPathParts = []string{"/oauth/", "/v1/oauth", "/api/oauth", "/api/auth/", "/login"}

// IsAccount reports whether a request is an Anthropic login or account request.
func IsAccount(host, path string) bool {
	h := strings.ToLower(hostOnly(host))
	for _, account := range accountHosts {
		if h == account || strings.HasSuffix(h, "."+account) {
			return true
		}
	}
	p := strings.ToLower(path)
	for _, part := range accountPathParts {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func hostOnly(authority string) string {
	if host, _, err := net.SplitHostPort(authority); err == nil {
		return host
	}
	return authority
}

// CatchAll is a loopback proxy that terminates TLS with a probe-only CA and forwards nothing.
type CatchAll struct {
	*Receiver
	// CAPEM is the probe CA certificate, for NODE_EXTRA_CA_CERTS in the probe child only. The
	// CA key never leaves this process's memory.
	CAPEM  []byte
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	mu     sync.Mutex
	leaves map[string]*tls.Certificate
}

// ListenCatchAll starts the proxy around o.
func ListenCatchAll(o *Observer) (*CatchAll, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "clauduct provider probe CA"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, MaxPathLenZero: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	c := &CatchAll{CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}}
	r, err := listen(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.serveProxy(w, r, o) }), nil, nil, o)
	if err != nil {
		return nil, err
	}
	c.Receiver = r
	return c, nil
}

func (c *CatchAll) serveProxy(w http.ResponseWriter, r *http.Request, o *Observer) {
	if r.Method != http.MethodConnect {
		// An absolute-form plain HTTP request through the proxy.
		o.observe(w, r, r.Host)
		return
	}
	authority := r.Host
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no hijack", http.StatusInternalServerError)
		return
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		conn.Close()
		return
	}
	var inner net.Conn = conn
	if buffered != nil && buffered.Reader.Buffered() > 0 {
		inner = &prefixedConn{Conn: conn, prefix: buffered.Reader}
	}
	// Every tunnel is recorded, with its authority, before anything is read from it: a tunnel
	// the probe cannot see into must not vanish from the record.
	tunnel := o.record(Seen{At: time.Now(), Host: hostOnly(authority), Authority: authority, Method: http.MethodConnect, PathClass: "connect"})
	tlsConn := tls.Server(inner, &tls.Config{NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := hello.ServerName
			if name == "" {
				name = hostOnly(authority)
			}
			return c.leaf(name)
		}})
	handshake, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = tlsConn.HandshakeContext(handshake)
	cancel()
	if err != nil {
		// A client that sent bytes the probe could not decrypt: refused pin, h2-only, distrust.
		o.update(tunnel, func(s *Seen) { s.Undecodable = true })
		tlsConn.Close()
		return
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { o.observe(w, r, authority) }), ReadHeaderTimeout: 10 * time.Second}
	_ = server.Serve(newOneConn(tlsConn))
}

func (c *CatchAll) leaf(name string) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cert, ok := c.leaves[name]; ok {
		return cert, nil
	}
	cert, _, err := issue([]string{name}, c.caCert, c.caKey)
	if err != nil {
		return nil, err
	}
	c.leaves[name] = &cert
	return &cert, nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

// issue makes a leaf for names, signed by parent or by itself when parent is nil.
func issue(names []string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	template := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: names[0]},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(2 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, name)
		}
	}
	signer, signerKey := template, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, template, signer, &key.PublicKey, signerKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, der, nil
}

// oneConn is a listener that yields one connection, then blocks until that connection closes.
type oneConn struct {
	conn net.Conn
	once sync.Once
	done chan struct{}
}

func newOneConn(conn net.Conn) *oneConn { return &oneConn{conn: conn, done: make(chan struct{})} }

func (l *oneConn) Accept() (net.Conn, error) {
	var conn net.Conn
	l.once.Do(func() { conn = &closeNotify{Conn: l.conn, done: l.done} })
	if conn != nil {
		return conn, nil
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *oneConn) Close() error   { return nil }
func (l *oneConn) Addr() net.Addr { return l.conn.LocalAddr() }

type closeNotify struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

func (c *closeNotify) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}

type prefixedConn struct {
	net.Conn
	prefix *bufio.Reader
}

func (c *prefixedConn) Read(b []byte) (int, error) {
	if c.prefix != nil && c.prefix.Buffered() > 0 {
		return c.prefix.Read(b)
	}
	return c.Conn.Read(b)
}

// Scan returns the IDs of the canaries in r, in any of the searched forms, reading at most
// maxScannedBody bytes. It is for bodies the probe reads itself, such as INTENDED's backend
// calls.
func Scan(r io.Reader, canaries []Canary) ([]string, error) { return scan(r, canaries) }
