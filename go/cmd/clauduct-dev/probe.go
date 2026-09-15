package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// probe answers one question that cannot be answered offline: does the backend accept
// max_output_tokens?
//
// The Node baseline never sends it and the PoC recorded a rejection, but "recorded once"
// and "rejected today" are different claims, and the answer decides whether the client's
// max_tokens can ever be a cap on generation or only a check after the fact. Measuring it
// costs two real requests on the cheapest route, which is what the user authorised.
//
// Everything about this is deliberately narrow. It sends a two-word prompt, reads only the
// event types and the reported counts, prints no model output, and refuses to run at all
// without --send. A probe that can be triggered by a typo is a probe that spends money by
// accident.

// probePrompt has to want to produce more than probeLimit allows.
//
// A two-word answer would finish inside the cap either way, and both a backend that
// honoured the cap and one that ignored it would send response.completed — the measurement
// would come back identical in both worlds. Counting to forty is a few hundred tokens on
// the cheapest route and makes the difference visible.
const probePrompt = "Count from 1 to 40, separated by commas. Numbers only."

// probeInstruction is the developer turn. The baseline always sends one, and the fixed
// top-level instructions string points at it, so a request without one is not the request
// this bridge actually makes.
const probeInstruction = "You are a test fixture. Answer exactly what is asked, nothing else."

// probeLimits are the caps under test, and there are two of them for a reason.
//
// A single value cannot tell "the parameter is refused" from "that value is below the
// minimum". Sixteen is the smallest value the public Responses API documents; forty-eight
// is comfortably above any plausible floor and still well under what the prompt produces,
// so a backend that accepts the parameter at all has to truncate at it.
var probeLimits = []int{16, 48}

// The probes and what each one costs. A name is required because "probe" alone must not
// choose which question to spend money on.
var probes = map[string]struct {
	measures string
	attempts int
}{
	"limit": {"whether the backend accepts max_output_tokens", 3},
	"wire":  {"whether this module's own request encoding survives the real backend", 3},
}

func usageProbe(out io.Writer) int {
	budget := upstream.ApprovedBudget()
	fmt.Fprintln(out, "probe sends real requests and spends real money. Nothing is sent without --send.")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  route      %s at %s effort\n", budget.Model, budget.Effort)
	fmt.Fprintf(out, "  authorised %d attempts cumulatively, 2026-09-15\n", budget.Limit)
	fmt.Fprintf(out, "  endpoint   %s\n", upstream.Endpoint)
	fmt.Fprintln(out)
	for _, name := range probeNames() {
		p := probes[name]
		fmt.Fprintf(out, "  %-6s %d attempts — %s\n", name, p.attempts, p.measures)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "run: clauduct-dev probe <name> --send")
	return 2
}

// probeNames lists the probes in a fixed order. Map iteration is random and this text is
// read by a person deciding what to spend.
func probeNames() []string { return []string{"limit", "wire"} }

func probe(args []string, out, errOut io.Writer) int {
	if len(args) != 2 || args[1] != "--send" {
		return usageProbe(out)
	}
	selected, known := probes[args[0]]
	if !known {
		return usageProbe(out)
	}

	provider := &auth.Provider{}
	// Checked before anything is read: a machine logging TLS keys is one where this
	// request's bytes are readable afterwards, and a probe is not worth that.
	if err := provider.CheckRuntime(); err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", auth.CategoryOf(err))
		return 1
	}
	if err := provider.CheckHome(); err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", auth.CategoryOf(err))
		return 1
	}

	version, err := upstream.InstalledVersion()()
	if err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", err)
		return 1
	}
	fmt.Fprintf(out, "client  codex-cli %s (%s)\n", version, upstream.Status(version))

	budget := upstream.ApprovedBudget()
	budget.Limit = selected.attempts
	ledger := upstream.NewLedger(budget)
	transport := upstream.NewDirect(provider, ledger, upstream.Fixed(version), budget.Model, budget.Effort)

	code := 0
	if args[0] == "wire" {
		code = wireProbe(transport, budget, out)
	} else {
		code = limitProbe(transport, budget, out)
	}
	attempts, inferences, refused := ledger.Spent()
	fmt.Fprintf(out, "spent   %d attempts, %d inferences, %d refused\n", attempts, inferences, refused)
	return code
}

func limitProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) int {
	// The control first. If the shape the baseline has always sent does not work today,
	// nothing the other cases report means anything.
	control := send(transport, budget, 0)
	fmt.Fprintf(out, "%-28s %s\n", "no max_output_tokens", control)

	var capped []outcome
	for _, limit := range probeLimits {
		result := send(transport, budget, limit)
		capped = append(capped, result)
		fmt.Fprintf(out, "%-28s %s\n", fmt.Sprintf("max_output_tokens: %d", limit), result)
	}

	fmt.Fprintf(out, "reading %s\n", verdict(control, capped))

	// A backend that refuses the parameter is an answer, not a failure. What would make
	// this run worthless is the control not working.
	if !control.ok {
		return 1
	}
	return 0
}

// verdict states what the run established. It is the only place the results are compared,
// so the reasoning sits here rather than in whoever reads the output.
//
// The capped cases are read in order and the first informative one wins: one value being
// refused says nothing on its own, because it may simply be below a minimum, but one value
// being honoured settles the question for all of them.
func verdict(control outcome, capped []outcome) string {
	if !control.ok {
		return "INVALID — the baseline request shape did not work, so nothing else here says anything"
	}
	if len(capped) == 0 {
		return "INCONCLUSIVE — nothing was sent with the parameter"
	}

	refusedAll := true
	for i, result := range capped {
		limit := probeLimits[i]
		switch {
		case result.ok && result.reason == "max_output_tokens":
			return fmt.Sprintf("HONOURED at %d — the backend stopped at the cap. "+
				"A pre-generation limit is available", limit)
		case result.ok && result.outputTokens > int64(limit):
			return fmt.Sprintf("IGNORED at %d — accepted and then exceeded. "+
				"The parameter is not a cap", limit)
		case result.ok:
			return fmt.Sprintf("INCONCLUSIVE — accepted at %d but the answer fit inside it, "+
				"so the cap was never reached", limit)
		}
		if result.status != 400 {
			refusedAll = false
		}
	}
	if refusedAll {
		return "REJECTED — the backend refuses max_output_tokens at every value tried, " +
			"so this is the parameter and not a minimum. The baseline is right not to send it"
	}
	return "INCONCLUSIVE — the capped requests failed for reasons other than the parameter"
}

// outcome is what one probe request established. Every field is a number or a value from a
// fixed vocabulary: nothing the backend wrote is carried into it.
type outcome struct {
	ok           bool
	category     string
	status       int
	outputTokens int64
	terminal     string
	reason       string
}

func (o outcome) String() string {
	if !o.ok {
		if o.status != 0 {
			return fmt.Sprintf("REFUSED %s (HTTP %d)", o.category, o.status)
		}
		return "REFUSED " + o.category
	}
	text := fmt.Sprintf("accepted, %s, %d output tokens", o.terminal, o.outputTokens)
	if o.reason != "" {
		text += ", reason " + o.reason
	}
	return text
}

// send makes one request and reports what came back.
//
// limit of zero omits max_output_tokens, which is the shape the baseline sends. A non-zero
// limit is the thing being measured.
func send(transport upstream.Transport, budget upstream.Budget, limit int) outcome {
	body := map[string]any{
		"model":        budget.Model,
		"instructions": "Follow the developer instructions in the conversation.",
		"input": []any{
			map[string]any{"role": "developer", "content": probeInstruction},
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": probePrompt}},
			},
		},
		"reasoning": map[string]any{"effort": budget.Effort},
		"include":   []string{"reasoning.encrypted_content"},
		"stream":    true,
		"store":     false,
	}
	if limit > 0 {
		body["max_output_tokens"] = limit
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return outcome{category: "PROBE_BODY"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	response, err := transport.Execute(ctx, encoded)
	if err != nil {
		var failure upstream.Failure
		if errors.As(err, &failure) {
			return outcome{category: failure.Category, status: failure.Status}
		}
		if category := auth.CategoryOf(err); category != "" {
			return outcome{category: category}
		}
		return outcome{category: err.Error()}
	}
	defer response.Body.Close()
	return read(response)
}

// read walks the stream for event types and counts. The text the model produced is never
// looked at, let alone printed: what is being measured is whether the request was accepted
// and where generation stopped.
func read(response *upstream.Response) outcome {
	parser := stream.NewParser(stream.DefaultLimits())
	buffer := make([]byte, 32*1024)
	result := outcome{ok: true, terminal: "no terminal event"}

	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			events, parseErr := parser.Push(buffer[:n])
			if parseErr != nil {
				return outcome{category: parseErr.Error()}
			}
			for _, event := range events {
				switch event.Type {
				case codex.Completed:
					result.terminal = "response.completed"
					if usage, err := codex.DecodeUsage(event.Raw); err == nil {
						result.outputTokens = usage.OutputTokens
					}
				case codex.Incomplete:
					// This is the answer the probe exists for: the backend stopped where
					// it was told to, and the reason says so.
					result.terminal = "response.incomplete"
					result.reason = incompleteReason(event.Raw)
					if usage, err := codex.DecodeUsage(event.Raw); err == nil {
						result.outputTokens = usage.OutputTokens
					}
				case codex.Failed:
					result.ok = false
					result.terminal = "response.failed"
				case codex.ErrorEvent:
					result.ok = false
					result.terminal = "error event"
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return result
			}
			return outcome{category: upstream.ClassifyTransport(err).Category}
		}
	}
}

// incompleteReason reads the stated reason out of a response.incomplete payload, accepting
// only the values the wire is known to use. An unrecognised one is reported as such rather
// than echoed: this string is printed, and nothing the backend writes is printed verbatim.
func incompleteReason(raw []byte) string {
	var payload struct {
		Response struct {
			Details struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return "unreadable"
	}
	for _, known := range []string{"max_output_tokens", "max_tokens", "content_filter", "steered"} {
		if payload.Response.Details.Reason == known {
			return known
		}
	}
	return "unrecognised"
}
