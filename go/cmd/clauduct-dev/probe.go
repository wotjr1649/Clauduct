package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
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

// probeAttempts is what one --send run may spend. The cumulative authorisation is twenty;
// this bounds a single invocation so that repeating the command is a visible decision
// rather than a way to drift past the cap without noticing.
const probeAttempts = 3

// probePrompt is the shortest thing that still requires the model to generate. Anything
// longer costs more and measures the same thing.
const probePrompt = "reply ok"

// probeLimit is the cap under test. Sixteen is small enough that a backend honouring it
// must truncate a normal reply, which is what makes the difference observable.
const probeLimit = 16

func usageProbe(out io.Writer) int {
	budget := upstream.ApprovedBudget()
	fmt.Fprintln(out, "probe sends real requests and spends real money. Nothing is sent without --send.")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  route      %s at %s effort\n", budget.Model, budget.Effort)
	fmt.Fprintf(out, "  this run   up to %d attempts\n", probeAttempts)
	fmt.Fprintf(out, "  authorised %d attempts cumulatively, 2026-09-15\n", budget.Limit)
	fmt.Fprintf(out, "  endpoint   %s\n", upstream.Endpoint)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  measures   whether the backend accepts max_output_tokens")
	fmt.Fprintln(out, "  prompt     "+strconv.Quote(probePrompt))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "run: clauduct-dev probe --send")
	return 2
}

func probe(args []string, out, errOut io.Writer) int {
	if len(args) != 1 || args[0] != "--send" {
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

	version, err := installedCodexVersion()
	if err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", err)
		return 1
	}
	status := "unverified"
	if version == referenceCodexVersion {
		status = "reference"
	}
	fmt.Fprintf(out, "client  codex-cli %s (%s)\n", version, status)

	budget := upstream.ApprovedBudget()
	budget.Limit = probeAttempts
	ledger := upstream.NewLedger(budget)
	transport := upstream.NewDirect(provider, ledger, version, budget.Model, budget.Effort)

	// The control first. If the shape the baseline has always sent does not work today,
	// nothing the second case reports means anything.
	failures := 0
	for _, run := range []struct {
		label string
		limit int
	}{
		{"without max_output_tokens", 0},
		{"with max_output_tokens", probeLimit},
	} {
		result := send(transport, budget, run.limit)
		fmt.Fprintf(out, "%-26s %s\n", run.label, result)
		if !result.ok {
			failures++
		}
	}

	attempts, inferences, refused := ledger.Spent()
	fmt.Fprintf(out, "spent   %d attempts, %d inferences, %d refused\n", attempts, inferences, refused)
	return failures
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
		"input": []any{map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": probePrompt}},
		}},
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
			return outcome{category: failure.Category}
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
				case codex.Failed:
					result.ok = false
					result.terminal = "response.failed"
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

// referenceCodexVersion is the version this project measured the wire against. It is
// evidence, not a pin: a different installed version still runs, and is reported as
// unverified so the difference is in front of whoever reads the result.
const referenceCodexVersion = "0.153.4"

var codexVersionLine = regexp.MustCompile(`^codex-cli (\S+)$`)

// A version is what a request identifies itself as, so it is read from the installed
// executable rather than chosen here, and it is found through the same resolver that finds
// claude.exe — a version taken from a directory an attacker can prepend to PATH would let
// them choose what this bridge claims to be.
func installedCodexVersion() (string, error) {
	path, found, err := platform.Resolver{}.Codex()
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("CODEX_NOT_FOUND")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Output, not CombinedOutput: a warning on stderr must not become part of a version
	// string that goes on the wire.
	raw, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", errors.New("CLI_VERSION_UNREADABLE")
	}
	return parseCodexVersion(string(raw))
}

// parseCodexVersion reads a version out of what the executable printed.
//
// Separate from running it so that what goes on the wire can be tested against output this
// machine does not produce. Whatever is printed, only a short printable token is accepted:
// this value becomes a request header.
func parseCodexVersion(raw string) (string, error) {
	match := codexVersionLine.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil || !plausibleVersion(match[1]) {
		return "", errors.New("CLI_VERSION_INVALID")
	}
	return match[1], nil
}

// plausibleVersion bounds what may go into a header. A version is a short token with no
// whitespace; anything else is refused rather than sent.
func plausibleVersion(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}
