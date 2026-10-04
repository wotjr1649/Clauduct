package gateway

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// toolSurface records the tool names native offered this session that the re-measured
// native did not (#297). Clauduct runs a native nobody measured (owner's decision), and
// native adds and hides tools by model and environment; a name outside the measured set is
// reported, never refused. MCP and plugin tools (mcp__*) are the user's own and not compared.
type toolSurface struct {
	mu        sync.Mutex
	reference map[string]bool
	added     map[string]bool
}

var toolSurfaceName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// toolSurfaceNames bounds what a session keeps: the first names it met.
const toolSurfaceNames = 16

func newToolSurface(reference []string) *toolSurface {
	s := &toolSurface{reference: map[string]bool{}, added: map[string]bool{}}
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
	names := deferredTools(body)
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range names {
		if s.reference[name] || strings.HasPrefix(name, "mcp__") || !toolSurfaceName.MatchString(name) {
			continue
		}
		if len(s.added) < toolSurfaceNames {
			s.added[name] = true
		}
	}
}

var (
	deferredMarker = []byte("deferred tools are now available via ToolSearch")
	deferredStart  = []byte("before calling them:")
	escapedLine    = []byte(`\n`)
)

// deferredTools reads native's deferred-tool lists from the JSON body as sent: one name per
// escaped line after each list's introduction. A line that is not one whole name ends that
// list, so a sentence after it is never read as a tool; names with a hyphen (MCP) are read
// past and left to observe's filter.
func deferredTools(body []byte) []string {
	var names []string
	for rest := body; len(names) < 256; {
		at := bytes.Index(rest, deferredMarker)
		if at < 0 {
			break
		}
		rest = rest[at+len(deferredMarker):]
		start := bytes.Index(rest, deferredStart)
		if start < 0 || start > 512 {
			continue
		}
		list := rest[start+len(deferredStart):]
		for len(names) < 256 && bytes.HasPrefix(list, escapedLine) {
			line := list[len(escapedLine):]
			end := 0
			for end < len(line) && end < 128 && nameByte(line[end]) {
				end++
			}
			if end == 0 || !bytes.HasPrefix(line[end:], escapedLine) && !bytes.HasPrefix(line[end:], []byte(`"`)) {
				break
			}
			names = append(names, string(line[:end]))
			list = line[end:]
		}
	}
	return names
}

func nameByte(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b|0x20 >= 'a' && b|0x20 <= 'z'
}

func (s *toolSurface) report() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sorted(s.added)
}

var referenceTools = upstream.ReferenceTools
