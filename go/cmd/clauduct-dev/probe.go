package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
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
	// Two sends and two spare. The client's "0 cached" is a constant, not a reading --
	// DecodeUsage reads input_tokens and output_tokens and nothing else -- so before any
	// cache accounting can be built, the backend has to be asked whether it reports one.
	"cache": {"whether the backend reports cached input tokens", 2},
	// The follow-up, separate because it is a different question and a reader deciding what
	// to spend should see the two prices apart. Measured 2026-09-18: cache lands as
	// input_tokens_details.cached_tokens and both it and cache_write_tokens read zero over
	// a 4,325-token prefix -- and a cold call writing nothing is what a prefix under the
	// minimum looks like, so the size is the next thing to vary.
	"cache-long": {"whether a larger prefix is what the cache minimum wanted", 2},
	"wire":       {"whether this module's own request encoding survives the real backend", 3},
	// Four inferences and one search. The search is a different endpoint and is not an
	// attempt; it is named here so what is spent is visible before it is spent.
	"parity": {"whether images, tool changes, the reasoning round trip and search work", 4},
	// The search endpoint is not an inference and spends no attempt, so it can be run on
	// its own as often as diagnosing it takes.
	"search": {"whether the standalone search endpoint answers in the expected shape", 0},
	// One request, and the answer is entirely in the response headers. The body is read
	// and thrown away.
	"headers": {"whether the backend sends the rate limit headers D5 would read", 1},
	// One request. The client can attach a PDF and this build refuses it; whether that is
	// worth implementing depends on an answer only the backend has.
	"file": {"whether the backend reads an attached PDF at all", 1},
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
func probeNames() []string {
	return []string{"limit", "cache", "cache-long", "wire", "parity", "search", "headers", "file"}
}

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
	transport := upstream.NewDirect(provider, ledger, upstream.Fixed(version))

	code := 0
	switch args[0] {
	case "cache":
		code = cacheProbe(transport, budget, out, cacheRepeats)
	case "cache-long":
		code = cacheProbe(transport, budget, out, 4*cacheRepeats)
	case "wire":
		code = wireProbe(transport, budget, out)
	case "parity":
		code = parityProbe(transport, budget, out)
	case "headers":
		code = headersProbe(transport, budget, out)
	case "search":
		if !searchProbe(transport, budget, out) {
			code = 1
		}
	case "file":
		code = fileProbe(transport, budget, out)
	default:
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
	// usage is every integer the backend put in its usage object, including one level of
	// nesting, flattened as "parent.child". Read as a map rather than into a struct
	// because the question is which fields exist, and a struct can only find the ones
	// somebody already thought of.
	usage map[string]int64
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

	// The probe names a backend model outright, so requested and effective are the same
	// string and the source is "direct". Stating it rather than leaving it blank keeps the
	// ledger's record of a spending run as readable as the product's.
	response, err := transport.Execute(ctx, upstream.Call{
		Body:      encoded,
		Requested: budget.Model,
		Model:     budget.Model,
		Effort:    budget.Effort,
		Source:    "direct",
	})
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
					result.usage = usageCounts(event.Raw)
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

// --- cache: does the backend say anything about cached input? -----------------------------

// cachePrefix is the shared head of both cache requests.
//
// Long on purpose and identical on purpose. Prompt caching everywhere it exists has a
// minimum prefix length, and a short probe that found nothing could not tell "this backend
// does not report caching" from "that prompt was too small to cache" -- which is the same
// mistake as the single max_output_tokens value the limit probe deliberately avoids.
//
// Deterministic text rather than random: two requests have to carry byte-identical prefixes
// or there is nothing for a cache to hit.
func cachePrefix(repeats int) string {
	const line = "The quick brown fox jumps over the lazy dog while the patient stars wheel overhead. "
	var b strings.Builder
	for i := 0; i < repeats; i++ {
		fmt.Fprintf(&b, "%3d. %s", i, line)
	}
	return b.String()
}

// cacheRepeats is the first prefix size tried, about 4,300 tokens.
const cacheRepeats = 220

// usageCounts is every integer in the usage object, one level of nesting included.
//
// Read as a map because the question is which fields exist. A struct would find only the
// fields someone already knew to look for, and "we did not look" is exactly how the
// client's 0 cached became a constant that reads like a measurement.
//
// Key names are filtered rather than echoed. This map is printed, and a name arriving from
// the backend is not something to put on a terminal unexamined.
func usageCounts(raw []byte) map[string]int64 {
	var payload struct {
		Response struct {
			Usage map[string]json.RawMessage `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	out := map[string]int64{}
	for name, value := range payload.Response.Usage {
		if !plainName(name) {
			continue
		}
		var count int64
		if json.Unmarshal(value, &count) == nil {
			out[name] = count
			continue
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(value, &nested) != nil {
			continue
		}
		for inner, innerValue := range nested {
			if !plainName(inner) {
				continue
			}
			if json.Unmarshal(innerValue, &count) == nil {
				out[name+"."+inner] = count
			}
		}
	}
	return out
}

// plainName accepts the shape a field name has and refuses anything else, so what reaches
// the terminal is bounded by this function rather than by the backend.
func plainName(name string) bool {
	if len(name) == 0 || len(name) > 40 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// cacheProbe sends the same long prefix twice and reports what the backend said about it.
//
// Two sends, not one. One answers whether a cache field exists; it cannot answer whether
// the number in it is a measurement, because a field that is always zero and a field that
// is zero on a cold call look the same. The second call is the only thing that can tell
// those apart.
func cacheProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer, repeats int) int {
	prefix := cachePrefix(repeats)
	fmt.Fprintf(out, "%-28s %d characters\n", "shared prefix", len(prefix))

	cold := sendCache(transport, budget, prefix)
	fmt.Fprintf(out, "%-28s %s\n", "cold", cold)
	fmt.Fprintf(out, "%-28s %s\n", "  usage fields", fieldList(cold.usage))

	warm := sendCache(transport, budget, prefix)
	fmt.Fprintf(out, "%-28s %s\n", "repeat, identical prefix", warm)
	fmt.Fprintf(out, "%-28s %s\n", "  usage fields", fieldList(warm.usage))

	fmt.Fprintf(out, "reading %s\n", cacheVerdict(cold, warm))
	if !cold.ok || !warm.ok {
		return 1
	}
	return 0
}

func sendCache(transport upstream.Transport, budget upstream.Budget, prefix string) outcome {
	body := map[string]any{
		"model":        budget.Model,
		"instructions": "Follow the developer instructions in the conversation.",
		"input": []any{
			map[string]any{"role": "developer", "content": probeInstruction},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "input_text",
					// The prefix first, the ask last. A cache hit is on a shared head, so
					// anything that varies has to come after everything that does not.
					"text": prefix + "\n\nReply with exactly the word: ok",
				}},
			},
		},
		"reasoning": map[string]any{"effort": budget.Effort},
		"include":   []string{"reasoning.encrypted_content"},
		"stream":    true,
		// Left as the product sends it. A probe that turns on storage to find a cache would
		// be measuring a request this build never makes.
		"store": false,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return outcome{category: "PROBE_BODY"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	response, err := transport.Execute(ctx, upstream.Call{
		Body:      encoded,
		Requested: budget.Model,
		Model:     budget.Model,
		Effort:    budget.Effort,
		Source:    "direct",
	})
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

// fieldList renders the usage fields in a fixed order, so two runs can be compared by eye.
func fieldList(usage map[string]int64) string {
	if len(usage) == 0 {
		return "none reported"
	}
	names := make([]string, 0, len(usage))
	for name := range usage {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, usage[name]))
	}
	return strings.Join(parts, " ")
}

// cacheVerdict states what the pair established, and states plainly when it established
// nothing. "No cache field" and "a cache field that never moved" are different findings and
// only the first one closes the question.
func cacheVerdict(cold, warm outcome) string {
	if !cold.ok || !warm.ok {
		return "inconclusive: a request was refused, so nothing about caching was measured"
	}
	cached := cacheFields(cold, warm)
	if len(cached) == 0 {
		return "the backend reports no cache field at all; the client's \"0 cached\" cannot " +
			"become a measurement without one"
	}
	// Either call, not just the second. Measured 2026-09-18: the non-zero arrived on the
	// *first* call of the pair, because its prefix overlapped a run a few minutes earlier --
	// and a verdict that only looked at the second reported "stayed zero" over a reading of
	// 3,840. What closes the question is that the field is ever anything but zero; which
	// call it happened on is a fact about the probe, not about the backend.
	for _, name := range cached {
		if cold.usage[name] > 0 || warm.usage[name] > 0 {
			return fmt.Sprintf("%s read %d then %d: it is a measurement, not a constant, "+
				"and this build is discarding it",
				name, cold.usage[name], warm.usage[name])
		}
	}
	return fmt.Sprintf("cache fields exist (%s) and stayed zero across an identical prefix: "+
		"either this route does not cache or the prefix was under its minimum",
		strings.Join(cached, ", "))
}

// cacheFields names the usage fields that look like they count cached input.
func cacheFields(runs ...outcome) []string {
	seen := map[string]bool{}
	var names []string
	for _, run := range runs {
		for name := range run.usage {
			if !strings.Contains(name, "cach") || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
