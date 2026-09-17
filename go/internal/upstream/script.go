package upstream

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
)

// ErrScriptExhausted means a request arrived that the script has no answer for and no
// default. It is an error rather than a repeat of the last turn: a fixture that answers the
// same thing forever lets a test pass while the client loops.
var ErrScriptExhausted = errors.New("SCRIPT_EXHAUSTED")

// Script answers requests with scripted turns, matched by content.
//
// Fixture replays one body for every request, so nothing built on it can drive a tool round
// trip -- that takes at least two turns, and the second has to differ or the client never
// finishes.
//
// Matching by content rather than by arrival order is not refinement. Measured 2026-09-15:
// a single `claude -p` run sends more than one request, because the client generates a
// session title alongside the conversation. A script answering in order gave the tool call
// to the title request and the conversation got the wrong turn. The client multiplexes, so
// a fixture standing in for a backend has to as well.
type Script struct {
	// Turns are consumed in order among those whose matcher accepts the request.
	Turns []ScriptTurn
	// Default answers anything no turn matched. Empty makes an unmatched request an error,
	// which is the right default for a test that means to account for every request.
	Default string

	mu        sync.Mutex
	used      []bool
	requests  []string
	unmatched int
}

// ScriptTurn is one scripted answer and the requests it applies to.
type ScriptTurn struct {
	// When decides whether this turn answers a request. Nil matches anything.
	When func(body string) bool
	// SSE is the response body.
	SSE string
}

// Conversation matches the request carrying the client's tool definitions.
//
// That is what separates the real exchange from the client's side requests: a session title
// is generated without tools, and the conversation the user started carries all of them.
func Conversation(body string) bool { return strings.Contains(body, `"tools":[`) }

// SideRequest matches a request that carries no tools.
func SideRequest(body string) bool { return !Conversation(body) }

func (s *Script) Execute(ctx context.Context, call Call) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text := string(call.Body)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, text)
	if s.used == nil {
		s.used = make([]bool, len(s.Turns))
	}

	for i, turn := range s.Turns {
		if s.used[i] {
			continue
		}
		if turn.When != nil && !turn.When(text) {
			continue
		}
		s.used[i] = true
		return &Response{Body: io.NopCloser(&replay{source: turn.SSE})}, nil
	}

	s.unmatched++
	if s.Default == "" {
		return nil, ErrScriptExhausted
	}
	return &Response{Body: io.NopCloser(&replay{source: s.Default})}, nil
}

// Requests returns every request body in arrival order. A tool test's whole question is
// what the client sent back after running the tool, so the history is the evidence.
func (s *Script) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// Conversations returns only the requests carrying tool definitions.
func (s *Script) Conversations() []string {
	var out []string
	for _, request := range s.Requests() {
		if Conversation(request) {
			out = append(out, request)
		}
	}
	return out
}

// Calls reports how many requests arrived.
func (s *Script) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// Unmatched reports how many requests fell through to Default. It is evidence about the
// client rather than about this script: a number above zero means the client asked for
// something the test did not anticipate.
func (s *Script) Unmatched() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unmatched
}
