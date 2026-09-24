package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The wire probe drives the product path rather than a hand-written body.
//
// The limit probe built its JSON by hand, so what it established was that the endpoint
// accepts that shape -- not that this module produces it. bridge.BuildRequest was never
// executed. Everything WP03 and WP04 encode on the request side (tools, tool_choice, the
// developer turn, recorded calls and results) has only ever been checked against fixtures,
// and a fixture agrees with whatever wrote it.
//
// That is the same exposure max_output_tokens had, and it cost one live call to find.

// probeTool is the only way to answer probeToolPrompt. A tool the model could talk its way
// around would leave "no call arrived" meaning either a wire defect or a model that chose
// not to bother, and those must not look the same.
const probeTool = `{"name":"get_build_token",` +
	`"description":"Returns this build's verification token. This is the only way to obtain it; ` +
	`it cannot be guessed, derived, or recalled.",` +
	`"input_schema":{"type":"object","properties":{"reason":{"type":"string",` +
	`"description":"why the token is needed"}},"required":["reason"]}}`

const probeToolPrompt = "What is this build's verification token? Use the tool."

// probeToolResult is what the tool would have returned. Nothing reads it but the model.
const probeToolResult = "BUILD-TOKEN-7Q4M"

func wireProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) int {
	head := probeHead(budget, 2048)

	// 1. Text. Establishes that what BuildRequest produces is accepted at all, which every
	//    later case depends on.
	text := roundTrip(transport, out, "text",
		head+`"messages":[{"role":"user","content":"Reply with the word ok."}]}`)
	if !text.ok {
		fmt.Fprintln(out, "reading INVALID — the product request shape was refused, "+
			"so nothing below says anything")
		return 1
	}

	// 2. A tool definition, and a question only the tool can answer.
	call := roundTrip(transport, out, "tool call",
		head+`"tools":[`+probeTool+`],`+
			`"tool_choice":{"type":"tool","name":"get_build_token"},`+
			`"messages":[{"role":"user","content":`+quote(probeToolPrompt)+`}]}`)
	if !call.ok || call.callID == "" {
		fmt.Fprintln(out, "reading INCONCLUSIVE — no tool call came back, so the recorded-call "+
			"shape was never exercised")
		return 1
	}

	// 3. The call and its result sent back as history. This is the encoding WP04 built and
	//    the one nothing has ever checked against a backend: function_call and
	//    function_call_output, tied by call_id.
	result := roundTrip(transport, out, "tool result", toolHistory(head, call))

	fmt.Fprintf(out, "reading %s\n", wireVerdict(text, call, result))
	if !result.ok {
		return 1
	}
	return 0
}

// toolHistory is the conversation that carries a tool call and its result back.
func toolHistory(head string, call exchange) string {
	return head + `"tools":[` + probeTool + `],"messages":[` +
		`{"role":"user","content":` + quote(probeToolPrompt) + `},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":` + quote(call.callID) +
		`,"name":"get_build_token","input":` + call.arguments + `}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":` + quote(call.callID) +
		`,"content":[{"type":"text","text":` + quote(probeToolResult) + `}]}]}` +
		`]}`
}

// exchange is what one round trip established. Like outcome, every field is a count, a
// fixed value, or something this build itself sent -- never the model's words.
type exchange struct {
	ok       bool
	category string
	// thought is the envelope a redacted_thinking block carried. This build wrote it and
	// nothing decrypts or prints it; keeping it is what lets the next turn send it back,
	// which is the only way to find out whether the backend accepts its own record.
	thought   string
	status    int
	frames    int
	kinds     []string
	callID    string
	arguments string
	// events and items are what actually arrived, recorded before anything is translated
	// so that a refusal still reports the wire rather than only the verdict. Both are type
	// names checked against a fixed vocabulary before being printed -- never the payloads.
	events []string
	items  []string
	// eventCounts and itemCounts count every event and every finished output item by type
	// (#85's first step: what the backend emits, before refusing what nobody reads). Items
	// are counted from response.output_item.done because the measured completion carries an
	// empty output array. usage is the completion's usage fields.
	eventCounts map[string]int
	itemCounts  map[string]int
	usage       map[string]int64
	// request is the backend request that was sent, kept for the local token count.
	request *bridge.Request
	// echoed reports whether the reply contains the token the tool returned. It is a
	// boolean derived from the reply, not the reply: the one thing that establishes the
	// result actually reached the model is that it could say something it had no other way
	// to know.
	echoed bool
	// reply accumulates the text so the search runs once over the whole thing.
	//
	// Deltas arrive in fragments, and a sixteen-character token splits across them as
	// readily as not. Testing each delta on its own reported PARTIAL for a round trip that
	// had in fact worked -- a probe that cannot see the answer is not evidence about the
	// answer. Bounded because it is a buffer holding backend bytes; nothing prints it.
	reply  []byte
	replyN int
}

func (e exchange) String() string {
	if !e.ok {
		text := "REFUSED " + e.category
		if e.status != 0 {
			text = fmt.Sprintf("REFUSED %s (HTTP %d)", e.category, e.status)
		}
		if len(e.events) > 0 {
			text += "\n               saw [" + strings.Join(e.events, " ") + "]"
		}
		if len(e.items) > 0 {
			text += "\n               output items [" + strings.Join(e.items, " ") + "]"
		}
		return text
	}
	text := fmt.Sprintf("%d frames [%s]", e.frames, strings.Join(e.kinds, " "))
	if len(e.items) > 0 {
		text += ", output items [" + strings.Join(e.items, " ") + "]"
	}
	if e.callID != "" {
		text += fmt.Sprintf(", call_id %d chars, arguments %s", len(e.callID), e.arguments)
	}
	if e.thought != "" {
		text += fmt.Sprintf(", thought %d chars", len(e.thought))
	}
	text += fmt.Sprintf(", reply %d chars", e.replyN)
	if e.echoed {
		text += ", tool result reached the model"
	}
	return text
}

// probeHead is the request prefix every probe shares.
//
// The effort is stated rather than left out, and that is a correction. Without it
// SelectRoute supplies the model's catalogue default -- max for luna -- while the approved
// budget is luna at low. The wire probe was spending at max effort under an approval for
// low, and reasoning tokens are most of what a call costs. Nothing caught it until the
// route began being authorised against the body that will actually be sent.
func probeHead(budget upstream.Budget, maxTokens int) string {
	return fmt.Sprintf(`{"model":%q,"max_tokens":%d,"stream":true,`+
		`"output_config":{"effort":%q},`+
		`"system":"You are a verification fixture. Be brief.",`,
		budget.Model, maxTokens, budget.Effort)
}

// roundTrip runs one request through the whole product path: decode the Anthropic request,
// build the backend request with bridge.BuildRequest, send it, and drive the real
// Translator over the reply.
//
// Nothing here hand-writes a backend body or hand-reads a backend event. A defect anywhere
// in that chain shows up as a refusal rather than as a probe that quietly agrees with
// itself.
func roundTrip(transport upstream.Transport, out io.Writer, label, requestJSON string,
	options ...anthropic.Options) exchange {
	return routedTrip(transport, out, label, requestJSON, nil, options...)
}

// routedTrip is roundTrip with a route that replaces the one the request resolves to, so a
// model the table does not route yet goes through the same product path.
func routedTrip(transport upstream.Transport, out io.Writer, label, requestJSON string,
	route []bridge.Route, options ...anthropic.Options) exchange {

	request, err := anthropic.DecodeRequest([]byte(requestJSON), options...)
	if err != nil {
		result := exchange{category: "REQUEST_DECODE: " + err.Error()}
		fmt.Fprintf(out, "%-14s %s\n", label, result)
		return result
	}
	backend, err := bridge.BuildRequest(request, route...)
	if err != nil {
		result := exchange{category: "BUILD_REQUEST: " + err.Error()}
		fmt.Fprintf(out, "%-14s %s\n", label, result)
		return result
	}
	body, err := json.Marshal(backend)
	if err != nil {
		result := exchange{category: "ENCODE: " + err.Error()}
		fmt.Fprintf(out, "%-14s %s\n", label, result)
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	response, err := transport.Execute(ctx, upstream.Call{
		Body:      body,
		Requested: request.Model,
		Model:     backend.Model,
		Effort:    backend.Effort.Effort,
		Source:    backend.Source,
	})
	if err != nil {
		result := exchange{category: categoryOf(err)}
		var failure upstream.Failure
		if errors.As(err, &failure) {
			result.status = failure.Status
		}
		fmt.Fprintf(out, "%-14s %s\n", label, result)
		return result
	}
	defer response.Body.Close()

	result := translate(response, request)
	result.request = backend
	fmt.Fprintf(out, "%-14s %s\n", label, result)
	return result
}

func categoryOf(err error) string {
	var failure upstream.Failure
	if errors.As(err, &failure) {
		return failure.Category
	}
	return err.Error()
}

// translate drives the real Translator over the reply and reports what it produced.
func translate(response *upstream.Response, request *anthropic.Request) exchange {
	translator := bridge.NewTranslatorFor(request, "")
	parser := stream.NewParser(stream.DefaultLimits())
	parser.IsTerminal = codex.Terminal // as the gateway parses it (messages.go)
	buffer := make([]byte, 32*1024)

	result := exchange{ok: true, eventCounts: map[string]int{}, itemCounts: map[string]int{}}
	seen := map[string]bool{}
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			events, err := parser.Push(buffer[:n])
			if err != nil {
				result.category = "SSE: " + err.Error()
				result.ok = false
				return result
			}
			for _, event := range events {
				// Recorded before translating, so a refusal still reports what arrived.
				if !seen["ev:"+event.Type] {
					seen["ev:"+event.Type] = true
					result.events = append(result.events, safeName(event.Type))
				}
				result.eventCounts[safeName(event.Type)]++
				switch event.Type {
				case codex.OutputItemDone:
					var done struct {
						Item struct {
							Type string `json:"type"`
						} `json:"item"`
					}
					if json.Unmarshal(event.Raw, &done) == nil {
						result.itemCounts[safeName(done.Item.Type)]++
					}
				case codex.Completed, codex.Incomplete:
					result.usage = usageCounts(event.Raw)
				}
				if event.Type == codex.Completed {
					result.items = completedItems(event.Raw)
				}
				frames, err := translator.Accept(event)
				if err != nil {
					result.category = "TRANSLATE: " + err.Error()
					result.ok = false
					return result
				}
				for _, frame := range frames {
					result.frames++
					if !seen[frame.Type] {
						seen[frame.Type] = true
						result.kinds = append(result.kinds, frame.Type)
					}
					readFrame(frame, &result)
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				result.echoed = bytes.Contains(result.reply, []byte(probeToolResult))
				result.reply = nil
				return result
			}
			result.category = upstream.ClassifyTransport(readErr).Category
			result.ok = false
			return result
		}
	}
}

// readFrame pulls the two things the probe needs out of a client frame: the identity of a
// tool call, and whether the token the tool returned came back.
//
// The call id and arguments are this build's own material on the way back -- the id was
// minted by the backend but is structural, and the arguments are checked for shape rather
// than printed as prose. The reply text is never kept, only tested for one fixed string.
func readFrame(frame anthropic.Frame, result *exchange) {
	var body struct {
		ContentBlock struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			Data  string          `json:"data"`
		} `json:"content_block"`
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
	}
	if json.Unmarshal(frame.Data, &body) != nil {
		return
	}
	if body.ContentBlock.Type == "tool_use" && result.callID == "" {
		result.callID = body.ContentBlock.ID
		result.arguments = argumentShape(body.ContentBlock.Input)
	}
	if body.ContentBlock.Type == "redacted_thinking" && result.thought == "" {
		result.thought = body.ContentBlock.Data
	}
	if body.Delta.Text != "" {
		result.replyN += len(body.Delta.Text)
		if len(result.reply) < maxReplyScan {
			result.reply = append(result.reply, body.Delta.Text...)
		}
	}
}

// argumentShape describes the arguments without reproducing them. What matters is that
// they are a JSON object carrying the declared property, not what the model wrote in it.
func argumentShape(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "not an object"
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		if name == "reason" {
			names = append(names, "reason")
			continue
		}
		names = append(names, "?")
	}
	return "{" + strings.Join(names, ",") + "}"
}

// wireVerdict states what the three round trips established.
func wireVerdict(text, call, result exchange) string {
	switch {
	case !text.ok:
		return "INVALID — the product request shape was refused"
	case call.callID == "":
		return "INCONCLUSIVE — no tool call arrived"
	case !result.ok:
		return "BROKEN at the recorded call — the backend accepted a new tool call but " +
			"refused the conversation carrying it back. function_call or " +
			"function_call_output is encoded wrongly: " + result.category
	case !result.echoed:
		return "PARTIAL — the recorded call was accepted but the tool's result did not " +
			"reach the model, so function_call_output carries nothing usable"
	default:
		return "SOUND — request, tool call and recorded result all survive the real backend"
	}
}

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// completedItems reports the kinds of item the completed output carried, through this
// build's own decoder. If the decoder cannot read the output that is itself the finding.
func completedItems(raw []byte) []string {
	items, present, err := codex.DecodeCompletedOutput(raw)
	if err != nil {
		return []string{"undecodable"}
	}
	if !present {
		return []string{"no output array"}
	}
	if len(items) == 0 {
		return []string{"empty output array"}
	}
	kinds := make([]string, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, safeName(item.Type))
	}
	return kinds
}

// safeName bounds what a backend-supplied type name can put on this terminal. The names are
// printed and read by a person; a name is a short lowercase identifier and anything else is
// reported as such rather than echoed.
func safeName(value string) string {
	if value == "" {
		return "(empty)"
	}
	if len(value) > 64 {
		return "(oversized)"
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return "(unprintable)"
		}
	}
	return value
}

// maxReplyScan bounds the buffer the token is looked for in. Far more than enough to find a
// short token near the start of an answer, and a fixed ceiling on backend bytes held.
const maxReplyScan = 64 * 1024
