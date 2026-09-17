package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// watchedTransport reports when the relay let go of the backend response.
//
// Asking the client whether it was released says nothing useful, because the client is the
// thing that stopped reading. What matters is whether the session is still holding an
// upstream request open, and the body being closed is the only honest signal for that.
type watchedTransport struct {
	sse    string
	closed chan struct{}
	once   sync.Once
}

func (w *watchedTransport) Execute(context.Context, upstream.Call) (*upstream.Response, error) {
	// Handed over in small pieces on purpose. A reader that returns the whole answer in
	// one call makes the relay emit it as one enormous batch, and then a single write is
	// megabytes long -- which a per-write bound will refuse, correctly, at a slow enough
	// client. A real backend arrives in network-sized chunks and the batches stay small.
	return &upstream.Response{Body: &watchedBody{
		Reader:  &chunked{source: w.sse, size: 2048},
		release: func() { w.once.Do(func() { close(w.closed) }) },
	}}, nil
}

// chunked hands out at most size bytes per read.
type chunked struct {
	source string
	size   int
	at     int
}

func (c *chunked) Read(p []byte) (int, error) {
	if c.at >= len(c.source) {
		return 0, io.EOF
	}
	end := c.at + c.size
	if end > len(c.source) {
		end = len(c.source)
	}
	n := copy(p, c.source[c.at:end])
	c.at += n
	return n, nil
}

type watchedBody struct {
	io.Reader
	release func()
}

func (b *watchedBody) Close() error {
	b.release()
	return nil
}

// longStream builds a response of parts text deltas of 8 KB each, which at two hundred
// parts is well past anything the socket buffers will absorb.
//
// The ceiling is the parser's, not this function's: MaxFrameBytes is 8 MiB and the closing
// snapshot event carries the whole text in one frame, so parts must stay below a thousand
// or the stream is refused before any of this is exercised.
func longStream(parts int) string {
	chunk := strings.Repeat("x", 8192)
	lines := []string{created}
	for i := 0; i < parts; i++ {
		lines = append(lines, delta(chunk))
	}
	lines = append(lines, done(strings.Repeat(chunk, parts)), completed, "[DONE]")
	return sse(lines...)
}

// shortStall lowers the write bound for one test. Three seconds exercises the same
// mechanism as thirty and keeps the suite quick enough that nobody skips it.
//
// Restored by cleanup, which runs after the gateway's own cleanup closes it down, so no
// handler is still reading this when it changes back.
func shortStall(t *testing.T) {
	t.Helper()
	previous := writeStall
	writeStall = 3 * time.Second
	t.Cleanup(func() { writeStall = previous })
}

// WIRE13. A client that stops reading must not hold the session open indefinitely.
//
// Nothing in this gateway bounded a write before: no WriteTimeout on the server, and a
// read deadline does not touch the write side. So the socket buffer filled, the next write
// blocked, and the goroutine, the backend connection and a request still running on the
// user's subscription stayed held for as long as the client cared to stay silent.
func TestAClientThatStopsReadingIsLetGoOf(t *testing.T) {
	shortStall(t)
	transport := &watchedTransport{sse: longStream(200), closed: make(chan struct{})}
	g := startWith(t, transport)

	// Headers arrive, then nothing is read. do() registers the body close for cleanup.
	post(t, g, validRequest)

	started := time.Now()
	select {
	case <-transport.closed:
	case <-time.After(writeStall + 25*time.Second):
		t.Fatalf("the session still held the backend request %v after the client stopped "+
			"reading; nothing bounds a blocked write", time.Since(started))
	}
	elapsed := time.Since(started)

	// Released early means the answer fit in the buffers and no write ever blocked, so
	// this test would be proving nothing about the bound.
	if elapsed < writeStall {
		t.Fatalf("released after %v, inside the %v bound. No write blocked, so this says "+
			"nothing about what happens when one does.", elapsed, writeStall)
	}
	t.Logf("a silent client held the session for %v against a %v bound",
		elapsed.Round(time.Millisecond), writeStall)
}

// And the bound must not be a response timeout wearing a different name: it has to be
// reset before every write, not set once for the response.
//
// Measured at the writer rather than through a socket. Two earlier versions of this test
// tried to prove it over TCP, with a client that reads slowly and one that pauses between
// gulps, and both were unreliable for the same reason: how long a write blocks depends on
// receive-buffer size and on when the peer bothers to advertise a reopened window, neither
// of which this code controls. One version even failed on correct code. The property is
// about ordering, so it is checked as ordering.
func TestTheBoundIsResetBeforeEveryWrite(t *testing.T) {
	g := start(t)
	// One byte at a time, so the parser hands over a batch per event and the response
	// really is written in several goes. A single-batch response cannot tell the two
	// designs apart.
	fixture := &upstream.Fixture{
		SSE:       sse(created, delta("a"), delta("b"), delta("c"), done("abc"), completed, "[DONE]"),
		ChunkSize: 1,
	}
	response, err := fixture.Execute(context.Background(), upstream.Call{})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	request, err := anthropic.DecodeRequest([]byte(validRequest))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	w := &recordingWriter{}
	g.relay(context.Background(), w, http.NewResponseController(w), response, request)

	flushes := 0
	for at, event := range w.events {
		if event != "flush" {
			continue
		}
		flushes++
		if at == 0 || w.events[at-1] != "bound" {
			t.Fatalf("flush %d went out without a fresh bound. The sequence was %v, which "+
				"is one deadline covering several writes -- a response timeout, not a "+
				"write timeout.", flushes, w.events)
		}
	}
	if flushes < 2 {
		t.Fatalf("the response was written in %d flush(es); with fewer than two this test "+
			"cannot tell a per-write bound from one set once. Sequence: %v", flushes, w.events)
	}
	if w.events[len(w.events)-1] != "clear" {
		t.Fatalf("the deadline outlived the response. Sequence: %v. It lives on the "+
			"connection, so the next request on it inherits what is left.", w.events)
	}
}

// recordingWriter keeps the order of the two things this test is about. Frame bytes are
// counted and dropped; what matters is that a bound precedes every flush.
type recordingWriter struct {
	header http.Header
	events []string
	bytes  int
}

func (r *recordingWriter) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	r.bytes += len(p)
	return len(p), nil
}

func (r *recordingWriter) WriteHeader(int) {}

func (r *recordingWriter) Flush() { r.events = append(r.events, "flush") }

func (r *recordingWriter) SetWriteDeadline(at time.Time) error {
	if at.IsZero() {
		r.events = append(r.events, "clear")
	} else {
		r.events = append(r.events, "bound")
	}
	return nil
}

// The shipped bound, asserted where a test cannot quietly replace it.
//
// shortStall overrides writeStall for the test above, so it says nothing about the value
// this actually ships with: raising it to twenty-four hours left that test green. A bound
// nobody checks is a bound that can stop being one.
func TestTheShippedWriteBoundIsActuallyABound(t *testing.T) {
	if writeStall <= 0 {
		t.Fatalf("writeStall = %v, so nothing bounds a write", writeStall)
	}
	if writeStall < 5*time.Second {
		t.Fatalf("writeStall = %v. A client that paused briefly would be cut off.", writeStall)
	}
	if writeStall > 2*time.Minute {
		t.Fatalf("writeStall = %v. A client that stopped reading would hold a goroutine, "+
			"the backend connection and a running request for that long.", writeStall)
	}
}

// A frame larger than one chunk must be written under more than one deadline.
//
// The bound says "one write may block for writeStall". That is only a statement about a
// bounded amount of data if the writes are bounded, and they are not by default: when the
// backend delivers faster than the parser drains, a single batch reaches megabytes. The
// WIRE13 measurements hit exactly that -- a live client cut off for the sender's pacing.
func TestALargeFrameIsWrittenUnderMoreThanOneBound(t *testing.T) {
	g := start(t)
	// Ten chunks' worth of text in one delta, delivered whole so the parser produces it
	// as a single frame. Anything less than two chunks would pass on unchunked writes.
	const parts = 10
	fixture := &upstream.Fixture{SSE: sse(created,
		delta(strings.Repeat("x", parts*writeChunk)),
		done(strings.Repeat("x", parts*writeChunk)), completed, "[DONE]")}
	response, err := fixture.Execute(context.Background(), upstream.Call{})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	request, err := anthropic.DecodeRequest([]byte(validRequest))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	w := &recordingWriter{}
	g.relay(context.Background(), w, http.NewResponseController(w), response, request)

	bounds := 0
	for _, event := range w.events {
		if event == "bound" {
			bounds++
		}
	}
	// The text alone is ten chunks, and it is carried in one frame. Anything close to the
	// number of frames means the writes were not split.
	if bounds < parts {
		t.Fatalf("%d bounds for a response carrying at least %d chunks of text in one "+
			"frame. A write that large cannot finish inside the bound, so the bound would "+
			"refuse a client that was keeping up. Sequence: %v", bounds, parts, w.events)
	}
	if w.bytes < parts*writeChunk {
		t.Fatalf("only %d bytes written, want at least %d", w.bytes, parts*writeChunk)
	}
	t.Logf("%d bytes went out under %d bounds of %d bytes each", w.bytes, bounds, writeChunk)
}
