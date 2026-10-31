package providercert

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

// A handler record proves one decoded request, not the rest of its TLS connection.
type tlsHTTPAccounting struct {
	mu             sync.Mutex
	bytes, decoded int64
	requests       int
	bad            bool
	wirePartial    atomic.Bool
	writer         *io.PipeWriter
	done           chan struct{}
}

func newTLSHTTPAccounting() *tlsHTTPAccounting {
	reader, writer := io.Pipe()
	a := &tlsHTTPAccounting{writer: writer, done: make(chan struct{})}
	go func() {
		defer close(a.done)
		defer reader.Close()
		counted := &tlsHTTPReader{Reader: reader}
		buffer := bufio.NewReader(counted)
		lastPost := false
		for {
			if lastPost {
				peek, _ := buffer.Peek(4)
				padding := 0
				for padding < len(peek) && (peek[padding] == '\r' || peek[padding] == '\n') {
					padding++
				}
				buffer.Discard(padding)
			}
			request, err := http.ReadRequest(buffer)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					a.fail()
				}
				return
			}
			_, err = io.Copy(io.Discard, request.Body)
			request.Body.Close()
			if err != nil {
				a.fail()
				return
			}
			a.mu.Lock()
			a.requests++
			a.decoded = counted.bytes - int64(buffer.Buffered())
			a.mu.Unlock()
			lastPost = request.Method == http.MethodPost
		}
	}()
	return a
}

type tlsHTTPReader struct {
	io.Reader
	bytes int64
}

func (r *tlsHTTPReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += int64(n)
	return n, err
}

func (a *tlsHTTPAccounting) Write(p []byte) (int, error) {
	a.mu.Lock()
	a.bytes += int64(len(p))
	a.mu.Unlock()
	_, _ = a.writer.Write(p)
	return len(p), nil
}

func (a *tlsHTTPAccounting) fail() { a.mu.Lock(); a.bad = true; a.mu.Unlock() }

func (a *tlsHTTPAccounting) matches(requests int) bool {
	select {
	case <-a.done:
	default:
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.bad && !a.wirePartial.Load() && a.requests == requests && a.bytes == a.decoded
}

// Bound raw reads to a TLS record so TLS cannot hide a later observed record in read-ahead.
type tlsRecordConn struct {
	net.Conn
	account         *tlsHTTPAccounting
	header          [5]byte
	used, remaining int
}

func (c *tlsRecordConn) NetConn() net.Conn { return c.Conn }

func (c *tlsRecordConn) Read(p []byte) (int, error) {
	limit := c.remaining
	if limit == 0 {
		limit = len(c.header) - c.used
	}
	n, err := c.Conn.Read(p[:min(len(p), limit)])
	if c.remaining != 0 {
		c.remaining -= n
	} else {
		c.used += copy(c.header[c.used:], p[:n])
		if c.used == len(c.header) {
			c.remaining = int(c.header[3])<<8 | int(c.header[4])
			c.used = 0
		}
	}
	c.account.wirePartial.Store(c.used != 0 || c.remaining != 0)
	return n, err
}

type tlsHTTPListener struct {
	net.Listener
	config   *tls.Config
	observer *Observer
}

func (l *tlsHTTPListener) Accept() (net.Conn, error) {
	raw, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	counted := raw.(countingConn)
	a := newTLSHTTPAccounting()
	counted.accepted.http = a
	secured := tls.Server(&tlsRecordConn{Conn: raw, account: a}, l.config)
	l.observer.mu.Lock()
	l.observer.trackActivity(counted.drainOwner, 1, 0)
	l.observer.mu.Unlock()
	return &tlsHTTPConn{Conn: secured, account: a, reader: bufio.NewReaderSize(io.TeeReader(secured, a), 16<<10),
		observer: l.observer, owner: counted.drainOwner}, nil
}

type tlsHTTPConn struct {
	*tls.Conn
	account  *tlsHTTPAccounting
	reader   *bufio.Reader
	mu       sync.Mutex
	closing  atomic.Bool
	once     sync.Once
	err      error
	observer *Observer
	owner    receiverOwner
}

func (c *tlsHTTPConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, err := c.reader.Read(p)
	var timeout net.Error
	if err != nil && !errors.Is(err, io.EOF) && !c.closing.Load() && !(errors.As(err, &timeout) && timeout.Timeout()) {
		c.account.fail()
	}
	return n, err
}

func (c *tlsHTTPConn) Close() error {
	c.once.Do(func() {
		c.closing.Store(true)
		c.err = c.Conn.Close()
		c.mu.Lock()
		c.account.writer.Close()
		<-c.account.done
		c.mu.Unlock()
		c.observer.mu.Lock()
		c.observer.trackActivity(c.owner, -1, 0)
		c.observer.mu.Unlock()
	})
	return c.err
}
