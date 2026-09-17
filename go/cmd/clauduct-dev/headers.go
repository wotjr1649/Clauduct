package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// D5. Does this backend send rate limit headers at all, and under what names?
//
// The Node baseline reads six numeric fields off x-codex-{primary,secondary}-{used-percent,
// window-minutes,reset-at}, citing codex-rs/codex-api/src/rate_limits.rs. That is a name
// read out of someone else's source tree, and a reader for headers that never arrive is a
// reader that reports "missing" forever while looking like it works. One request settles it.
//
// What it prints: every header name, and values only for names shaped like a rate limit
// field. A response can carry a credential-bearing header and a probe that dumps values
// wholesale is a probe that writes one into a terminal.
var rateLimitField = regexp.MustCompile(`^x-[a-z0-9-]*-(primary|secondary)-(used-percent|window-minutes|reset-at)$`)

func headersProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) int {
	body, err := json.Marshal(map[string]any{
		"model":        budget.Model,
		"instructions": "Follow the developer instructions in the conversation.",
		"input": []any{
			map[string]any{"role": "developer", "content": probeInstruction},
			map[string]any{"role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "Say ok."}}},
		},
		"reasoning": map[string]any{"effort": budget.Effort},
		"include":   []string{"reasoning.encrypted_content"},
		"stream":    true,
		"store":     false,
	})
	if err != nil {
		fmt.Fprintln(out, "headers PROBE_BODY")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	response, err := transport.Execute(ctx, upstream.Call{
		Body: body, Requested: budget.Model, Model: budget.Model,
		Effort: budget.Effort, Source: "direct",
	})
	if err != nil {
		fmt.Fprintf(out, "headers FAILED %v\n", err)
		return 1
	}
	// Read and discard. The answer is in the headers, but a body left unread is a
	// connection left in a state the next request has to clean up.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()

	names := make([]string, 0, len(response.Header))
	for name := range response.Header {
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)

	fmt.Fprintf(out, "headers %d names\n", len(names))
	for _, name := range names {
		if rateLimitField.MatchString(name) {
			fmt.Fprintf(out, "  %-44s %s\n", name, response.Header.Get(name))
			continue
		}
		fmt.Fprintf(out, "  %s\n", name)
	}
	return 0
}
