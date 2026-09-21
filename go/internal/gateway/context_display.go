package gateway

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// Native keeps /context's rendered output and hidden Markdown in its transcript.
// Only a complete engine-authored, parent-linked command/output/meta triple is
// display-only. Text resemblance alone is never provenance. The native renderer,
// transcript, plugins, and permission decisions remain unchanged.
type displayTriple [3][32]byte
type displayRow struct {
	Type, Subtype, SessionID, Version, UUID, ParentUUID, Content string
	IsMeta                                                       bool
	CommandRun                                                   *struct{ Command, Args string }
	Message                                                      struct {
		Role    string
		Content json.RawMessage
	}
}
type displayHistory struct {
	mu                            sync.Mutex
	path                          string
	info                          os.FileInfo
	offset                        int64
	previous                      [2]displayRow
	triples                       map[displayTriple]int
	removed, requests, unreadable int64
	capacityExceeded              bool
}
type ContextDisplayReport struct {
	Source           string `json:"source"`
	RemovedBlocks    int64  `json:"removedBlocks"`
	Requests         int64  `json:"requests"`
	Unreadable       int64  `json:"unreadable"`
	ProvenReports    int    `json:"provenReports"`
	NativeDisplay    string `json:"nativeDisplay"`
	CapacityExceeded bool   `json:"capacityExceeded"`
}

func (g *Gateway) contextDisplayReport() ContextDisplayReport {
	d := &g.displays
	d.mu.Lock()
	defer d.mu.Unlock()
	return ContextDisplayReport{"native_transcript_provenance", d.removed, d.requests, d.unreadable, len(d.triples), "unchanged_local_history_estimates", d.capacityExceeded}
}
func displayHash(text string) [32]byte { return sha256.Sum256([]byte(strings.TrimSpace(text))) }

// The path comes only from the existing authenticated SessionStart receipt and
// stays under os.Root. No HTTP-provided path or directory scan is used. Resume
// rebuilds the proof from native records, without a second persisted body copy.
func (g *Gateway) stripContextDisplays(request *anthropic.Request, session string) {
	if g.contexts == nil || g.delegations == nil {
		return
	}
	if session == "" {
		var ready nativeTurnReceipt
		if found, err := g.readNativeReceipt("ready.json", &ready); err != nil || !found {
			return
		}
		session = ready.Session
	}
	g.contexts.mu.Lock()
	name := g.contexts.sessions[session]
	g.contexts.mu.Unlock()
	if name == "" {
		return
	}
	d := &g.displays
	d.mu.Lock()
	defer d.mu.Unlock()
	root, err := os.OpenRoot(g.delegations.projects)
	if os.IsNotExist(err) {
		// Nothing written here yet, which the transcript check below already treats as
		// nothing to read. Counting it as unreadable made every first request on a new
		// configuration directory look like a provenance failure.
		return
	}
	if err != nil {
		d.unreadable++
		return
	}
	defer root.Close()
	f, err := root.Open(name)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		d.unreadable++
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		d.unreadable++
		return
	}
	if d.path != name || d.info == nil || !os.SameFile(d.info, info) || info.Size() < d.offset {
		d.path = name
		d.offset = 0
		d.previous = [2]displayRow{}
		d.triples = map[displayTriple]int{}
	}
	if d.info == nil || d.offset < info.Size() || info.Size() != d.info.Size() || !info.ModTime().Equal(d.info.ModTime()) {
		if info.Size() == d.offset && d.info != nil && !info.ModTime().Equal(d.info.ModTime()) {
			d.offset = 0
			d.previous = [2]displayRow{}
			d.triples = map[displayTriple]int{}
		}
		if _, err = f.Seek(d.offset, io.SeekStart); err != nil {
			d.unreadable++
			return
		}
		// Read only newly appended bytes. Overlong records are skipped as a whole;
		// no request, encrypted reasoning, or report body is logged or retained.
		reader := bufio.NewReaderSize(io.LimitReader(f, 32<<20), 64<<10)
		var line []byte
		consumed := int64(0)
		oversized := false
		for {
			part, readErr := reader.ReadSlice('\n')
			consumed += int64(len(part))
			if len(line)+len(part) > 2<<20 {
				oversized = true
				line = nil
			} else if !oversized {
				line = append(line, part...)
			}
			if readErr == bufio.ErrBufferFull {
				continue
			}
			if readErr != nil {
				break
			} // an unfinished append is read on the next request
			d.offset += consumed
			consumed = 0
			var row displayRow
			if !oversized && json.Unmarshal(line, &row) == nil {
				if row.Subtype == "compact_boundary" {
					d.triples = map[displayTriple]int{}
					d.previous = [2]displayRow{}
				}
				version := clientAgent.FindStringSubmatch("claude-cli/" + row.Version)
				if row.SessionID == session && len(version) == 2 && version[1] == row.Version && (g.ClientVersion() == "" || row.Version == g.ClientVersion()) {
					d.observeDisplay(row)
				} else {
					d.previous = [2]displayRow{}
				}
			} else {
				d.previous = [2]displayRow{}
			}
			line = nil
			oversized = false
		}
		d.info = info
	}
	removed := removeDisplayTriples(request, d.triples)
	if removed > 0 {
		d.removed += int64(removed)
		d.requests++
	}
}
func (d *displayHistory) observeDisplay(row displayRow) {
	command, output := d.previous[0], d.previous[1]
	var text string
	// Native print mode stores a user command and output pair, without the
	// interactive renderer's extra Markdown record. Both require engine output.
	var priorText string
	prior := d.previous[1]
	if prior.Type == "user" && !prior.IsMeta && json.Unmarshal(prior.Message.Content, &priorText) == nil && strings.HasPrefix(priorText, "<command-name>/context</command-name>") &&
		prior.Version == row.Version &&
		row.Type == "system" && row.Subtype == "local_command" && row.CommandRun != nil && row.CommandRun.Command == "context" && (row.CommandRun.Args == "" || row.CommandRun.Args == "all") &&
		strings.HasPrefix(row.Content, "<local-command-stdout>") && strings.HasSuffix(row.Content, "</local-command-stdout>") && correlationShape.MatchString(prior.UUID) && row.ParentUUID == prior.UUID {
		d.rememberDisplay(displayTriple{displayHash(priorText), displayHash(row.Content), {}})
	}
	if row.Type == "user" && row.IsMeta && row.Message.Role == "user" && json.Unmarshal(row.Message.Content, &text) == nil && strings.HasPrefix(text, "## Context Usage\n") &&
		command.Version == row.Version && output.Version == row.Version &&
		command.Type == "system" && command.Subtype == "local_command" && strings.HasPrefix(command.Content, "<command-name>/context</command-name>") &&
		output.Type == "system" && output.Subtype == "local_command" && output.CommandRun != nil && output.CommandRun.Command == "context" && (output.CommandRun.Args == "" || output.CommandRun.Args == "all") &&
		strings.HasPrefix(output.Content, "<local-command-stdout>") && strings.HasSuffix(output.Content, "</local-command-stdout>") &&
		correlationShape.MatchString(command.UUID) && output.ParentUUID == command.UUID && correlationShape.MatchString(output.UUID) && row.ParentUUID == output.UUID {
		d.rememberDisplay(displayTriple{displayHash(command.Content), displayHash(output.Content), displayHash(text)})
	}
	// Hold at most two relevant display rows; never keep ordinary message bodies.
	var commandText string
	userCommand := row.Type == "user" && !row.IsMeta && json.Unmarshal(row.Message.Content, &commandText) == nil && strings.HasPrefix(commandText, "<command-name>/context</command-name>")
	if !userCommand && (row.Type != "system" || row.Subtype != "local_command") {
		row = displayRow{}
	}
	d.previous = [2]displayRow{d.previous[1], row}
}

func (d *displayHistory) rememberDisplay(key displayTriple) {
	// ponytail: at most 4096 distinct display hashes per uncompacted transcript;
	// preserve unproven input and report saturation instead of evicting provenance.
	if d.triples[key] == 0 && len(d.triples) >= 4096 {
		d.capacityExceeded = true
		return
	}
	d.triples[key]++
}
func removeDisplayTriples(req *anthropic.Request, known map[displayTriple]int) int {
	if len(known) == 0 {
		return 0
	}
	type location struct{ message, block int }
	type match struct {
		key       displayTriple
		positions []location
	}
	var matches []match
	occurrences := map[displayTriple]int{}
	var hashes [3][32]byte
	var locations [3]location
	run := 0
	for mi, m := range req.Messages {
		if m.Role != "user" {
			run = 0
			continue
		}
		for bi, b := range m.Blocks {
			if b.Type != "text" {
				run = 0
				continue
			}
			hashes = [3][32]byte{hashes[1], hashes[2], displayHash(b.Text)}
			locations = [3]location{locations[1], locations[2], {mi, bi}}
			run++
			key := displayTriple(hashes)
			if run >= 3 && known[key] > 0 {
				occurrences[key]++
				matches = append(matches, match{key, append([]location(nil), locations[:]...)})
				run = 0
			} else if pair := (displayTriple{hashes[1], hashes[2], {}}); run >= 2 && known[pair] > 0 {
				occurrences[pair]++
				matches = append(matches, match{pair, []location{locations[1], locations[2]}})
				run = 0
			}
		}
	}
	remove := map[location]bool{}
	for _, m := range matches {
		// An extra identical copy may be a user's quotation. Ambiguous copies are
		// all preserved, rather than guessing which came from the command.
		if occurrences[m.key] != known[m.key] {
			continue
		}
		for _, p := range m.positions {
			remove[p] = true
		}
	}
	if len(remove) == 0 {
		return 0
	}
	out := make([]anthropic.Message, 0, len(req.Messages))
	for mi, m := range req.Messages {
		blocks := make([]anthropic.Block, 0, len(m.Blocks))
		for bi, b := range m.Blocks {
			if !remove[location{mi, bi}] {
				blocks = append(blocks, b)
			}
		}
		if len(blocks) > 0 {
			m.Blocks = blocks
			out = append(out, m)
		}
	}
	req.Messages = out
	return len(remove)
}
