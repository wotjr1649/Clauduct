package gateway

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

const localCount = "local-text-count-v1"
const backendCount = "backend-count-warmup"

type countedInput struct {
	tokens        int64
	at            time.Time
	model, method string
}

// Only digests, counts and catalogue identifiers are retained. Concurrent callers
// for identical input share one count; cancellation of a waiter leaves the owner alone.
// ponytail: bounded cache with arbitrary eviction; no LRU without hit-rate evidence.
type countCache struct {
	mu         sync.Mutex
	entries    map[[32]byte]countedInput
	pending    map[[32]byte]*countFlight
	quarantine map[string]string
}

type countFlight struct {
	done   chan struct{}
	tokens int64
	method string
	err    error
}

func (g *Gateway) countInput(ctx context.Context, built *bridge.Request, raw []byte, requested string) (result int64, source, origin string, failure error) {
	c := &g.counts
	key := sha256.Sum256(raw)
	for {
		c.mu.Lock()
		if value, found := c.entries[key]; found && value.method == "backend-usage" && time.Since(value.at) < 2*time.Minute {
			c.mu.Unlock()
			return value.tokens, "exact-usage-cache", value.method, nil
		}
		if c.quarantine[built.Model] == backendCount {
			c.mu.Unlock()
			return 0, backendCount, backendCount, upstream.Failure{Category: "COUNT_SOURCE_QUARANTINED"}
		}
		if value, found := c.entries[key]; found && time.Since(value.at) < 2*time.Minute {
			c.mu.Unlock()
			return value.tokens, "exact-count-cache", value.method, nil
		}
		if flight := c.pending[key]; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return 0, "exact-count-wait", "", ctx.Err()
			case <-flight.done:
				c.mu.Lock()
				bad := c.quarantine[built.Model]
				c.mu.Unlock()
				if bad == backendCount || bad != "" && bad == flight.method {
					return 0, "exact-count-shared", flight.method, upstream.Failure{Category: "COUNT_SOURCE_QUARANTINED"}
				}
				return flight.tokens, "exact-count-shared", flight.method, flight.err
			}
		}
		if c.pending == nil {
			c.pending = make(map[[32]byte]*countFlight)
		}
		flight := &countFlight{done: make(chan struct{})}
		c.pending[key] = flight
		localAllowed := c.quarantine[built.Model] != localCount
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			delete(c.pending, key)
			flight.tokens, flight.method, flight.err = result, origin, failure
			close(flight.done)
			c.mu.Unlock()
		}()
		method := localCount
		tokens, err := int64(0), bridge.ErrTokenCountUnsupported
		counter, backendAvailable := g.transport.(upstream.InputCounter)
		if backendAvailable && bridge.BackendCountSupported(built) {
			// The subscription backend is the oracle, including newly deployed
			// tokenizers. Never silently fall back after this exact count fails.
			method = backendCount
			tokens, err = counter.Count(ctx, upstream.Call{Body: raw, Requested: requested, Model: built.Model, Effort: built.Effort.Effort, Source: built.Source})
		} else if localAllowed {
			tokens, err = bridge.CountInput(built)
		}
		if err == nil && tokens <= 0 {
			err = upstream.ErrCountFailed
		}
		if err != nil {
			return 0, method, method, err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		// Generation can finish while this independent count is in flight.
		// An older warmup must never overwrite the actual usage for these bytes.
		if measured, ok := c.entries[key]; ok && measured.method == "backend-usage" && time.Since(measured.at) < 2*time.Minute {
			if measured.tokens != tokens {
				if c.quarantine == nil {
					c.quarantine = make(map[string]string)
				}
				c.quarantine[built.Model] = method
				return 0, method, method, upstream.Failure{Category: "COUNT_INPUT_MISMATCH"}
			}
			return measured.tokens, "exact-usage-cache", measured.method, nil
		}
		// A parallel generation may have invalidated this counter while we waited.
		if bad := c.quarantine[built.Model]; bad == backendCount || bad == method {
			return 0, method, method, upstream.Failure{Category: "COUNT_SOURCE_QUARANTINED"}
		}
		if c.entries == nil {
			c.entries = make(map[[32]byte]countedInput)
		}
		if len(c.entries) >= 128 {
			for old := range c.entries {
				delete(c.entries, old)
				break
			}
		}
		c.entries[key] = countedInput{tokens: tokens, at: time.Now(), model: built.Model, method: method}
		return tokens, method, method, nil
	}
}

// Compare only a count already available for these exact bytes. Generation never
// starts an additional inference/count request just to measure its own input.
func (g *Gateway) priorCount(raw []byte, r *record) {
	key := sha256.Sum256(raw)
	g.counts.mu.Lock()
	value, found := g.counts.entries[key]
	g.counts.mu.Unlock()
	if found && time.Since(value.at) < 2*time.Minute {
		r.preflight(value.tokens, "prior-count-cache", 0)
		r.countMethod(value.method)
	}
}

func (g *Gateway) rememberUsage(raw []byte, r *record) {
	data := r.snapshot()
	if data.InputTokens == nil {
		return
	}
	c := &g.counts
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[[32]byte]countedInput)
	}
	if len(c.entries) >= 128 {
		for key := range c.entries {
			delete(c.entries, key)
			break
		}
	}
	c.entries[sha256.Sum256(raw)] = countedInput{tokens: *data.InputTokens, at: time.Now(), model: data.Model, method: "backend-usage"}
}

// The completed provider usage is the oracle. A mismatch invalidates all cached
// counts for that model. Local drift moves subsequent counts to the backend;
// backend drift blocks subsequent counts rather than claiming exactness.
func (g *Gateway) verifyCount(r *record) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.data.CountAgreement != "" || r.data.CountedInputTokens == nil || r.data.InputTokens == nil {
		r.mu.Unlock()
		return nil
	}
	match := *r.data.CountedInputTokens == *r.data.InputTokens
	r.data.CountAgreement = "matched"
	if !match {
		r.data.CountAgreement = "mismatched"
	}
	model, method := r.data.Model, r.data.CountMethod
	r.mu.Unlock()
	if match {
		return nil
	}
	c := &g.counts
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, count := range c.entries {
		if count.model == model {
			delete(c.entries, key)
		}
	}
	if c.quarantine == nil {
		c.quarantine = make(map[string]string)
	}
	if c.quarantine[model] != backendCount {
		c.quarantine[model] = method
	}
	return upstream.Failure{Category: "COUNT_INPUT_MISMATCH"}
}
