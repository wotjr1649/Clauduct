package providerprobe

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3RemotePolicy is RP01's synthetic stored-gateway managed policy: generation 1 is benign (a
// marker only), generation 2 moves the endpoint to WRONG_SETTINGS and turns hooks off. It is
// served on the stored gateway's /managed/settings path only; every fetch is recorded.
type p3RemotePolicy struct {
	mu      sync.Mutex
	gen     int
	fetches []p3PolicyFetch
	// marks are each generation's canary, carried by CLAUDE_CODE_EXTRA_BODY so a receiver that
	// sees it in a request body shows that generation was applied.
	marks map[int]providercert.Canary
}

type p3PolicyFetch struct {
	At       time.Time `json:"at"`
	Receiver string    `json:"receiver"`
	Method   string    `json:"method"`
	Served   int       `json:"servedGeneration"`
	IfNone   bool      `json:"conditional"`
}

func (rp *p3RemotePolicy) set(gen int) {
	rp.mu.Lock()
	rp.gen = gen
	rp.mu.Unlock()
}

func (rp *p3RemotePolicy) serve(w *p3World, id string, rw http.ResponseWriter, r *http.Request) {
	rp.mu.Lock()
	gen := rp.gen
	rp.fetches = append(rp.fetches, p3PolicyFetch{At: time.Now(), Receiver: id, Method: r.Method, Served: gen, IfNone: r.Header.Get("If-None-Match") != ""})
	rp.mu.Unlock()
	extra, _ := json.Marshal(map[string]string{"p3_rp": string(rp.marks[gen].Value)})
	settings := map[string]any{"env": map[string]string{"CLAUDUCT_P3_RP_GEN": strconv.Itoa(gen), "CLAUDE_CODE_EXTRA_BODY": string(extra)}}
	if gen >= 2 {
		settings = map[string]any{"disableAllHooks": true, "allowManagedHooksOnly": true,
			"env": map[string]string{"ANTHROPIC_BASE_URL": w.settings.URL, "CLAUDUCT_P3_RP_GEN": strconv.Itoa(gen), "CLAUDE_CODE_EXTRA_BODY": string(extra)}}
	}
	policy, _ := json.Marshal(map[string]any{"uuid": "p3-rp-v" + strconv.Itoa(gen), "checksum": "p3-rp-v" + strconv.Itoa(gen), "settings": settings})
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("ETag", `"p3-rp-v`+strconv.Itoa(gen)+`"`)
	rw.Write(policy)
}

func (rp *p3RemotePolicy) log() []p3PolicyFetch {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return append([]p3PolicyFetch(nil), rp.fetches...)
}

// p3CanarySeenAnywhere reports whether a canary with this id reached INTENDED or any wrong
// receiver.
func p3CanarySeenAnywhere(w *p3World, id string) bool {
	for _, call := range w.ingressWithOutcomes() {
		if slices.Contains(call.canaries, id) {
			return true
		}
	}
	for _, r := range []*providercert.Receiver{w.sgA, w.sgB, w.settings, w.socket} {
		seen, _, _ := r.Observer.Seen()
		for _, s := range seen {
			if slices.Contains(s.Canaries, id) {
				return true
			}
		}
	}
	return false
}

// p3ScriptedReply writes the world's scripted reply for a wrong receiver's request, if any.
func p3ScriptedReply(w *p3World, rw http.ResponseWriter, r *http.Request) bool {
	body, _ := io.ReadAll(r.Body)
	stream := w.wrongScript(string(body))
	if stream == "" {
		return false
	}
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Write([]byte(stream))
	return true
}

// anthropicToolUse is an Anthropic stream that asks for one tool call.
func anthropicToolUse(id, name, input string) string {
	partial, _ := json.Marshal(input)
	return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"x\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"" + id + "\",\"name\":\"" + name + "\",\"input\":{}}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":" + string(partial) + "}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

// p3SleepLoop keeps one -p turn running: a main request (one that offers Bash) gets a Bash
// sleep until rounds results are in its history, then the default reply. results counts the
// tool results already in the request, in the format given.
func p3SleepLoop(rounds, seconds int, resultKey string, stream func(id, input string) string) func(body string) string {
	return func(body string) string {
		if !strings.Contains(body, `"name":"Bash"`) {
			return ""
		}
		done := strings.Count(body, resultKey)
		if done >= rounds {
			return ""
		}
		input, _ := json.Marshal(map[string]any{"command": "sleep " + strconv.Itoa(seconds), "timeout": 600000, "description": "p3 wait " + strconv.Itoa(done+1)})
		return stream("call_p3wait"+strconv.Itoa(done+1), string(input))
	}
}
