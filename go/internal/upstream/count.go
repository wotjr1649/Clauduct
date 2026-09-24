package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
)

var ErrCountUnsupported = errors.New("COUNT_TOKENS_UNSUPPORTED")
var ErrCountFailed = errors.New("COUNT_TOKENS_FAILED")

// /context requests roughly fifty counts. Eight workers bound socket churn;
// the sixteen-worker live trial did not reduce the measured command latency.
const countConcurrency = 8

// InputCounter is optional: replay transports do not pretend to count inputs.
type InputCounter interface {
	Count(context.Context, Call) (int64, error)
}

// Count asks the existing subscription backend to prepare, but never generate,
// the exact Responses payload. The native WebSocket library shares our TLS and
// no-redirect HTTP client. Credentials, payloads and provider errors are not logged.
func (d *Direct) Count(ctx context.Context, call Call) (int64, error) {
	if d.Ledger == nil {
		return 0, ErrBudgetExhausted
	}
	if err := agreesWithBody(call); err != nil {
		return 0, err
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(call.Body, &payload) != nil || payload == nil {
		return 0, ErrCountUnsupported
	}
	delete(payload, "stream")
	payload["type"] = json.RawMessage(`"response.create"`)
	payload["generate"] = json.RawMessage(`false`)
	payload["store"] = json.RawMessage(`false`)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, ErrCountFailed
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d.counts.once.Do(func() { d.counts.slots = make(chan struct{}, countConcurrency) })
	select {
	case d.counts.slots <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-d.counts.slots }()
	if err := d.Ledger.Reserve(Attempt{Requested: call.Requested, Model: call.Model, Effort: call.Effort, Source: "backend-count", CountOnly: true}); err != nil {
		return 0, err
	}
	if d.Credentials == nil {
		return 0, &auth.Error{Category: auth.CategoryUnavailable}
	}
	credential, err := d.Credentials.Credential()
	if err != nil {
		return 0, err
	}
	if credential.Synthetic {
		return 0, ErrSyntheticMixing
	}
	version, err := d.clientVersion()
	if err != nil {
		return 0, err
	}
	headers, _ := http.NewRequest(http.MethodGet, d.target(), nil)
	applyHeaders(headers, credential, version, 0)
	headers.Header.Del("Content-Length")
	headers.Header.Del("Content-Type")
	headers.Header.Set("OpenAI-Beta", "responses_websockets=2026-02-06")
	// A connection carries its authentication and model scope. Rotation or a
	// model/effort switch must never reuse another scope's connection.
	key := sha256.Sum256([]byte(headers.Header.Get("Authorization") + "\x00" + credential.Account + "\x00" + version + "\x00" + call.Model + "\x00" + call.Effort))
	connection := d.counts.take(key)
	var response *http.Response
	if connection == nil {
		d.counts.dials.Add(1)
		connection, response, err = websocket.Dial(ctx, d.target(), &websocket.DialOptions{HTTPClient: d.client(), HTTPHeader: headers.Header, CompressionMode: websocket.CompressionDisabled})
	}
	if err != nil {
		if errors.Is(err, ErrRedirected) {
			return 0, ErrRedirected
		}
		if response != nil && response.StatusCode != http.StatusSwitchingProtocols {
			return 0, ClassifyStatus(response.StatusCode, response.Header, time.Now())
		}
		return 0, ClassifyTransport(err)
	}
	reusable := false
	defer func() {
		if reusable {
			d.counts.put(key, connection)
		} else {
			_ = connection.CloseNow()
		}
	}()
	connection.SetReadLimit(1 << 20)
	if err := connection.Write(ctx, websocket.MessageText, encoded); err != nil {
		return 0, ClassifyTransport(err)
	}
	for range 64 {
		kind, data, err := connection.Read(ctx)
		if err != nil {
			return 0, ClassifyTransport(err)
		}
		if kind != websocket.MessageText {
			return 0, ErrCountFailed
		}
		var event struct {
			Type     string `json:"type"`
			Response struct {
				Output []json.RawMessage `json:"output"`
				Usage  struct {
					Input  *int64 `json:"input_tokens"`
					Output *int64 `json:"output_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}
		if json.Unmarshal(data, &event) != nil {
			return 0, ErrCountFailed
		}
		switch event.Type {
		case "response.created", "response.in_progress", "response.metadata", "responsesapi.websocket_timing", codex.RateLimitsUpdated, codex.CodexRateLimits, codex.CodexMetadata:
		case "response.completed":
			usage := event.Response.Usage
			if usage.Input == nil || *usage.Input <= 0 || usage.Output == nil || *usage.Output != 0 || len(event.Response.Output) != 0 {
				return 0, ErrCountFailed
			}
			reusable = true
			return *usage.Input, nil
		default:
			return 0, ErrCountFailed
		}
	}
	return 0, ErrCountFailed
}

// A /context breakdown issues many independent counts in a burst. Reuse only
// completed count connections; every request still sends the entire input with
// generate=false/store=false and never uses previous_response_id. In-flight
// connections are exclusive, so parallel agents cannot consume each other's events.
// Idle lifetime is deliberately short; failed/cancelled sockets never re-enter.
type countConnections struct {
	mu            sync.Mutex
	idle          []*idleCountConnection
	closed        bool
	dials, reused atomic.Int64
	once          sync.Once
	slots         chan struct{}
}
type idleCountConnection struct {
	key   [32]byte
	conn  *websocket.Conn
	timer *time.Timer
}

func (p *countConnections) take(key [32]byte) *websocket.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, item := range p.idle {
		if item.key != key {
			continue
		}
		p.idle = append(p.idle[:i], p.idle[i+1:]...)
		item.timer.Stop()
		p.reused.Add(1)
		return item.conn
	}
	return nil
}

type CountConnectionStats struct {
	Dials  int64 `json:"dials"`
	Reused int64 `json:"reused"`
}

func (d *Direct) CountStats() CountConnectionStats {
	return CountConnectionStats{Dials: d.counts.dials.Load(), Reused: d.counts.reused.Load()}
}

func (p *countConnections) put(key [32]byte, conn *websocket.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.idle) >= countConcurrency {
		_ = conn.CloseNow()
		return
	}
	item := &idleCountConnection{key: key, conn: conn}
	p.idle = append(p.idle, item)
	item.timer = time.AfterFunc(2*time.Second, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		for i, idle := range p.idle {
			if idle == item {
				p.idle = append(p.idle[:i], p.idle[i+1:]...)
				_ = conn.CloseNow()
				break
			}
		}
	})
}

// Close releases idle network state at session shutdown. Active requests are
// cancelled by the gateway's registry and cannot return a socket to this pool.
func (d *Direct) Close() error {
	p := &d.counts
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, item := range p.idle {
		item.timer.Stop()
		_ = item.conn.CloseNow()
	}
	p.idle = nil
	return nil
}
