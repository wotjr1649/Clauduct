package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// toolSurface records the tool names native offered this session that the re-measured
// native did not (#297). Clauduct runs a native nobody measured (owner's decision), and
// native adds and hides tools by model and environment; a name outside the measured set is
// reported, never refused. MCP and plugin tools (mcp__*) are the user's own and not compared.
//
// A session can also be told the tools its mode should see (#300); those it never saw are
// reported as missing once any tools were offered at all.
//
// Two things are kept apart. The added and missing diagnostics read both the tool definitions
// and the deferred-tool text in the prompt (a diagnostic only: a user system prompt or message
// can write the same words). A positive match needs one request to define the complete core,
// no unmeasured non-MCP definition, and no omitted diagnostic names.
type toolSurface struct {
	mu                 sync.Mutex
	reference          map[string]bool
	added              map[string]bool
	expected           map[string]bool
	seen               map[string]bool
	offered            bool
	matchedRequest     bool
	definedOutside     bool
	addedOmitted       uint64
	deferredIncomplete bool
}

// toolSurfaceNames bounds what a session keeps: the first names it met.
const toolSurfaceNames = 16

// shortcut: count saturates at JavaScript's safe integer; widen the status format if needed.
const maxToolOmissions = 1<<53 - 1

func newToolSurface(reference []string) *toolSurface {
	s := &toolSurface{reference: map[string]bool{}, added: map[string]bool{}, seen: map[string]bool{}}
	for _, name := range reference {
		s.reference[name] = true
	}
	return s
}

// observe takes the tool definitions and, from the raw body, the names native lists as
// deferred: with ENABLE_TOOL_SEARCH (session.go) native sends a deferred tool as one line of
// a list in the prompt, not as a definition, and a tool a new native adds is likely one of
// those (measured 2.1.289, #297).
func (s *toolSurface) observe(tools []anthropic.Tool, body []byte) {
	if len(s.reference) == 0 {
		return // nothing was measured, so nothing can be outside it
	}
	names, incomplete := deferredTools(body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offered = s.offered || len(names)+len(tools) > 0
	s.deferredIncomplete = s.deferredIncomplete || incomplete
	defined := make(map[string]bool, len(s.expected))
	record := func(name string) {
		if s.expected[name] {
			s.seen[name] = true
		}
		if s.reference[name] || strings.HasPrefix(name, "mcp__") || !anthropic.ValidToolIdentifier(name) || s.added[name] {
			return
		}
		if len(s.added) == toolSurfaceNames {
			if s.addedOmitted < maxToolOmissions {
				s.addedOmitted++
			}
			return
		}
		s.added[name] = true
	}
	for _, tool := range tools {
		if s.expected[tool.Name] {
			defined[tool.Name] = true
		}
		if !s.reference[tool.Name] && !strings.HasPrefix(tool.Name, "mcp__") {
			s.definedOutside = true
		}
		record(tool.Name)
	}
	s.matchedRequest = s.matchedRequest || len(s.expected) > 0 && len(defined) == len(s.expected)
	for _, name := range names {
		record(name)
	}
}

// deferredNames bounds one body's deferred names. Far above native's own tools, so a long MCP
// list cannot push a core tool past it and have it reported missing (#300).
const (
	deferredNames     = 4096
	deferredNameBytes = 200
	deferredMarker    = "deferred tools are now available via ToolSearch"
	deferredStart     = "before calling them:"
)

// deferredTools decodes JSON strings before reading the diagnostic lists. A limit or invalid
// JSON makes the diagnostic incomplete; deferred names cannot positively establish Verified.
func deferredTools(body []byte) ([]string, bool) {
	if len(body) == 0 || !bytes.Contains(body, []byte(deferredMarker)) && !bytes.Contains(body, []byte(`\u`)) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var names []string
	incomplete := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return names, incomplete
		}
		if err != nil {
			return names, true
		}
		text, ok := token.(string)
		if !ok {
			continue
		}
		for rest := text; ; {
			at := strings.Index(rest, deferredMarker)
			if at < 0 {
				break
			}
			rest = rest[at+len(deferredMarker):]
			limit := min(len(rest), 512+len(deferredStart))
			start := strings.Index(rest[:limit], deferredStart)
			if start < 0 || start > 512 {
				continue
			}
			list := rest[start+len(deferredStart):]
			for strings.HasPrefix(list, "\n") || strings.HasPrefix(list, "\r\n") {
				line := strings.TrimPrefix(list, "\r")
				line = line[1:]
				end := 0
				for end < len(line) && end <= deferredNameBytes && nameByte(line[end]) {
					end++
				}
				if end > deferredNameBytes {
					incomplete = true
					break
				}
				if end == 0 || end < len(line) && line[end] != '\n' && !strings.HasPrefix(line[end:], "\r\n") {
					break
				}
				names = append(names, line[:end])
				if len(names) == deferredNames {
					return names, true
				}
				list = line[end:]
			}
		}
	}
}

func nameByte(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b|0x20 >= 'a' && b|0x20 <= 'z'
}

func (s *toolSurface) report() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sorted(s.added)
}

// expect sets the tools this session's mode should see; nil turns the comparison off.
func (s *toolSurface) expect(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expected = map[string]bool{}
	s.matchedRequest = false
	for _, name := range names {
		s.expected[name] = true
	}
}

// missing names the expected tools no request offered. A session that offered no tools at
// all (it ended first, or sent only auxiliary requests) has nothing to compare.
func (s *toolSurface) missing() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.missingLocked()
}

// missingLocked is missing for a caller that holds s.mu.
func (s *toolSurface) missingLocked() []string {
	if !s.offered {
		return nil
	}
	gone := map[string]bool{}
	for name := range s.expected {
		if !s.seen[name] {
			gone[name] = true
		}
	}
	return sorted(gone)
}

var referenceTools = upstream.ReferenceTools

// snapshot reads added, missing and whether this session's observation matched, under one lock
// so the three agree. added and missing are the diagnostics (tool definitions and deferred text).
// matched is the partial check behind Verified: one request must define every expected core
// name, no request may define an unmeasured non-MCP name, and no diagnostic name may be omitted.
// Deferred text never provides positive evidence. expect(nil), a narrowed argv or a background
// session leaves nothing to compare; measured non-core omissions, schemas and MCP are not verified.
func (s *toolSurface) snapshot() (added, missing []string, matched bool, addedOmitted uint64, deferredIncomplete bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	added, missing = sorted(s.added), s.missingLocked()
	return added, missing, s.matchedRequest && !s.definedOutside && s.addedOmitted == 0, s.addedOmitted, s.deferredIncomplete
}
