package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The parity probe asks the backend about the four things WP07's A group implemented.
//
// Every one of them passed offline, and offline agreement is what this project has twice
// found to be worth nothing on its own: a live call found the model name being forwarded
// raw, and another found max_output_tokens refused outright. An image that the backend will
// not read, or a search endpoint that answers in a shape this build does not expect, fails
// in exactly the way a fixture cannot show.

// onePixelPNG is the smallest thing that is actually an image. Generated, not taken from
// anywhere, and it carries no content of its own.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// parityProbe runs four inferences and one search.
func parityProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) int {
	head := probeHead(budget, 1024)
	failures := 0

	// 1. An image. A1 turns {source:{type:'base64',...}} into an input_image data URL, and
	//    whether this backend reads that shape is not something a fixture can answer.
	image := roundTrip(transport, out, "image",
		head+`"messages":[{"role":"user","content":[`+
			`{"type":"text","text":"Reply with the single word: image"},`+
			`{"type":"image","source":{"type":"base64","media_type":"image/png","data":`+
			quote(onePixelPNG)+`}}]}]}`)
	if !image.ok {
		failures++
	}

	// 2. A withdrawn tool. A4a drops it from the definitions that go upstream, so what this
	//    establishes is that the request is still well formed with the beta negotiated and
	//    one definition short.
	change := roundTrip(transport, out, "tool change",
		head+`"tools":[`+probeTool+`,`+
			`{"name":"unused_probe_tool","description":"never called",`+
			`"input_schema":{"type":"object"}}],`+
			`"messages":[{"role":"user","content":"Reply with the single word: ok"},`+
			`{"role":"system","content":[{"type":"tool_removal","tool":`+
			`{"type":"tool_reference","name":"unused_probe_tool"}}]}]}`,
		anthropic.Options{ToolChanges: true})
	if !change.ok {
		failures++
	}

	// 3 and 4. The reasoning round trip. The first turn is only worth anything if the
	//    backend actually returns an encrypted record; the second is the one that matters,
	//    because it hands that record back and finds out whether the backend accepts it.
	first := roundTrip(transport, out, "reasoning out",
		head+`"messages":[{"role":"user","content":"Name one prime between 10 and 20. One word."}]}`)
	if !first.ok {
		failures++
	}
	// Whichever turn came back with a record. Measured 2026-09-16: at low effort a trivial
	// prompt often returns none while a request carrying tools does, so looking only at the
	// turn named "reasoning out" reported NOT_RUN with a 2045-character record sitting in
	// the exchange above it.
	thought := first.thought
	for _, candidate := range []string{change.thought, image.thought} {
		if thought == "" {
			thought = candidate
		}
	}
	switch {
	case thought == "":
		fmt.Fprintln(out, "reasoning back NOT_RUN — no turn came back with an encrypted record, "+
			"so there was nothing to hand back. Not a pass.")
	default:
		back := roundTrip(transport, out, "reasoning back",
			head+`"messages":[`+
				`{"role":"user","content":"Name one prime between 10 and 20. One word."},`+
				`{"role":"assistant","content":[{"type":"redacted_thinking","data":`+
				quote(thought)+`}]},`+
				`{"role":"user","content":"And one between 20 and 30. One word."}]}`)
		if !back.ok {
			failures++
		}
	}

	// 5. The search endpoint. Not an inference and not counted as one: it is a different
	//    endpoint with its own semantics, and calling it an attempt would make a budget
	//    stated in attempts stop meaning that.
	if !searchProbe(transport, budget, out) {
		failures++
	}

	if failures > 0 {
		fmt.Fprintf(out, "reading %d of 5 failed\n", failures)
		return 1
	}
	fmt.Fprintln(out, "reading all five reached the backend and came back in the shape this "+
		"build expects")
	return 0
}

// searchProbe drives the real search endpoint through the same code the gateway uses.
func searchProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) bool {
	searcher, ok := transport.(upstream.Searcher)
	if !ok {
		fmt.Fprintln(out, "search         REFUSED this transport cannot search")
		return false
	}
	query := bridge.SearchQuery{Query: "go programming language release history"}
	body, err := json.Marshal(bridge.BuildSearchRequest(budget.Model, query))
	if err != nil {
		fmt.Fprintf(out, "search         REFUSED ENCODE %v\n", err)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	raw, err := searcher.Search(ctx, body)
	if err != nil {
		var failure upstream.Failure
		if errors.As(err, &failure) && failure.Status != 0 {
			fmt.Fprintf(out, "search         REFUSED %s (HTTP %d), %d bytes sent\n",
				failure.Category, failure.Status, len(body))
			return false
		}
		fmt.Fprintf(out, "search         REFUSED %s, %d bytes sent\n", categoryOf(err), len(body))
		return false
	}
	results, err := bridge.DecodeSearchResults(raw)
	if err != nil {
		fmt.Fprintf(out, "search         REFUSED %s (%d bytes back)\n", err, len(raw))
		return false
	}

	// Counts and shapes only. The links are web content and none of them is printed.
	hosts := map[string]bool{}
	for _, link := range results.Links {
		if i := strings.Index(link.URL, "://"); i >= 0 {
			rest := link.URL[i+3:]
			if j := strings.IndexAny(rest, "/?#"); j >= 0 {
				rest = rest[:j]
			}
			hosts[rest] = true
		}
	}
	frames := bridge.SearchFrames(budget.Model, query, results, bridge.SearchID)
	fmt.Fprintf(out, "search         %d bytes back, %d links over %d hosts, %d chars of text, "+
		"%d frames out\n", len(raw), len(results.Links), len(hosts), len(results.Output), len(frames))
	return true
}
