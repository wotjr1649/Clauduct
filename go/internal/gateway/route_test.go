package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// CAP06. A request that carries no correlation header is an ordinary request.
//
// The native client sends X-Claude-Code-Session-Id (measured, 1.1 of the validation note),
// and the baseline uses one to tie a subagent to its session. Nothing here needs it, and
// the distinction this fixes is that its absence must not be reported as the other thing
// that can go wrong with routing -- a model this build cannot resolve. One is not a
// problem at all; the other is the caller's to fix. They must not arrive as the same
// answer.
func TestAMissingCorrelationHeaderIsNotARoutingProblem(t *testing.T) {
	for _, carried := range []bool{false, true} {
		name := "without the header"
		if carried {
			name = "with the header"
		}
		t.Run(name, func(t *testing.T) {
			fixture := &upstream.Fixture{SSE: sse(created, delta("hi"), done("hi"), completed, "[DONE]")}
			g := startWith(t, fixture)

			rq := messages(strings.NewReader(validRequest))
			if carried {
				rq.headers["X-Claude-Code-Session-Id"] = "d1f0c0de-0000-4000-8000-000000000001"
			}
			resp := do(t, g, rq)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", resp.StatusCode, bodyText(t, resp))
			}
			if fixture.Calls() != 1 {
				t.Fatalf("backend calls = %d, want 1", fixture.Calls())
			}
		})
	}
}

// And the other one is refused in a way the caller can act on.
//
// It used to be a 500 REQUEST_CONVERSION_FAILED, which was wrong twice: it told the user
// nothing they could do, and this client retries every 5xx -- measured at eight requests a
// minute -- so a request that could never succeed was sent eight times.
func TestAModelThisBuildCannotRouteIsRefusedAsTheCallersProblem(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]")}
	g := startWith(t, fixture)

	resp := post(t, g, `{"model":"gpt-9-nonesuch","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"ping"}]}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. A 5xx here is retried by this client.", resp.StatusCode)
	}
	body := bodyText(t, resp)
	if !strings.Contains(body, "UNSUPPORTED_MODEL_OR_EFFORT") {
		t.Fatalf("body = %s, want the route refusal named", body)
	}
	// Named distinctly from the path refusal. A reader who cannot tell "no such endpoint"
	// from "no such model" cannot act on either.
	if strings.Contains(body, refuseRoute.category) {
		t.Fatalf("body = %s: a model refusal is reporting itself as a path refusal", body)
	}
	if fixture.Calls() != 0 {
		t.Fatalf("backend calls = %d; a model that cannot be routed must cost nothing "+
			"upstream", fixture.Calls())
	}
}

// HTTP12. The baseline's auxiliary service talks to this same gateway.
//
// agent-route.mjs posts subagent bindings to /clauduct/agents on ANTHROPIC_BASE_URL, with
// the session token. This build does not implement that endpoint, and the requirement is
// not that it does -- it is that such a request is never mistaken for a model request. It
// must not be forwarded upstream, must not spend an attempt, and must not be answered in a
// way that lets the caller believe it worked.
func TestAuxiliaryServiceTrafficIsNeverTakenForAModelRequest(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]")}
	ledger := upstream.NewLedger(upstream.Unlimited())
	g := startWith(t, fixture)

	for _, path := range []string{"/clauduct/agents", "/clauduct/agents?beta=true", "/v1/complete"} {
		resp := do(t, g, request{
			method:  http.MethodPost,
			path:    path,
			headers: map[string]string{"Content-Type": "application/json"},
			body:    strings.NewReader(`{"kind":"task-result","id":"t1"}`),
		})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("POST %s = %d, want 404: %s", path, resp.StatusCode, bodyText(t, resp))
		}
		if body := bodyText(t, resp); !strings.Contains(body, refuseRoute.category) {
			t.Fatalf("POST %s body = %s, want %s", path, body, refuseRoute.category)
		}
	}
	if fixture.Calls() != 0 {
		t.Fatalf("backend calls = %d; auxiliary traffic reached the model path", fixture.Calls())
	}
	if attempts, inferences, _ := ledger.Spent(); attempts != 0 || inferences != 0 {
		t.Fatalf("ledger spent %d attempts and %d inferences on traffic that is not an "+
			"inference at all", attempts, inferences)
	}
}

// CAP03 end to end. What the gateway hands the transport is the pair, not one value.
//
// The gateway is the only place that knows both: the client's name for the model and the
// backend model it resolved to. If it passes one, nothing downstream can recover the other.
func TestTheGatewayHandsOverBothTheRequestedAndTheEffectiveRoute(t *testing.T) {
	for _, c := range []struct {
		requested, model, effort, source string
	}{
		{"claude-opus-5", "gpt-5.6-sol", "xhigh", "family"},
		{"claude-opus-4-1", "gpt-5.6-sol", "xhigh", "family"},
		{"haiku", "gpt-5.6-luna", "max", "alias"},
		{"astra", "gpt-6-astra", "medium", "catalogue"},
		{"gpt-6-astra", "gpt-6-astra", "medium", "direct"},
	} {
		t.Run(c.requested, func(t *testing.T) {
			seen := make(chan upstream.Call, 1)
			g := startWith(t, &recordingTransport{
				sse:  sse(created, delta("hi"), done("hi"), completed, "[DONE]"),
				seen: seen,
			})

			resp := post(t, g, `{"model":"`+c.requested+`","max_tokens":16,"stream":true,
			  "messages":[{"role":"user","content":"ping"}]}`)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
			}

			call := <-seen
			if call.Requested != c.requested {
				t.Errorf("Requested = %q, want %q. The client's own name for the model is "+
					"the half a billing record cannot reconstruct.", call.Requested, c.requested)
			}
			if call.Model != c.model || call.Effort != c.effort {
				t.Errorf("effective route = %s/%s, want %s/%s", call.Model, call.Effort,
					c.model, c.effort)
			}
			if call.Source != c.source {
				t.Errorf("Source = %q, want %q. A reader who sees a surprising model needs "+
					"to know which rule produced it.", call.Source, c.source)
			}
			// And the body agrees with the declaration, which is what the transport
			// authorises against.
			if !strings.Contains(string(call.Body), `"model":"`+c.model+`"`) {
				t.Errorf("body does not name %s: %s", c.model, call.Body)
			}
		})
	}
}

type recordingTransport struct {
	sse  string
	seen chan upstream.Call
}

func (r *recordingTransport) Execute(_ context.Context, call upstream.Call) (*upstream.Response, error) {
	select {
	case r.seen <- call:
	default:
	}
	return (&upstream.Fixture{SSE: r.sse}).Execute(context.Background(), call)
}

// HTTP08 / A3. The client's model list comes from here, and it names what will run.
//
// Without it the user's /model picker is the client's built-in Anthropic list: names that
// do not exist on this backend. Picking "Opus 5" ran gpt-5.6-sol, "Sonnet 5" and
// "Haiku 4.5" both ran gpt-5.6-luna, and gpt-5.6-terra could not be reached at all.
func TestTheModelListNamesWhatWillActuallyRun(t *testing.T) {
	fixture := &upstream.Fixture{}
	g := startWith(t, fixture)

	resp := do(t, g, request{method: http.MethodGet, path: "/v1/models"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, bodyText(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	var list struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(bodyText(t, resp)), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Object != "list" {
		t.Errorf("object = %q, want list", list.Object)
	}

	// The published order, and every backend model this build can route to. terra is in it
	// because the whole point is that the picker can reach it.
	want := []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}
	if len(list.Data) != len(want) {
		t.Fatalf("%d models, want %d: %+v", len(list.Data), len(want), list.Data)
	}
	for i, id := range want {
		if list.Data[i].ID != id {
			t.Errorf("data[%d].id = %q, want %q", i, list.Data[i].ID, id)
		}
		if list.Data[i].Object != "model" || list.Data[i].OwnedBy != "openai" {
			t.Errorf("data[%d] = %+v", i, list.Data[i])
		}
	}

	// Every name it publishes must route. A list naming something this build refuses would
	// be an invitation to pick a model that then fails.
	for _, item := range list.Data {
		if _, err := bridge.SelectRoute(item.ID, ""); err != nil {
			t.Errorf("the list offers %q but routing it fails: %v", item.ID, err)
		}
	}
	if fixture.Calls() != 0 {
		t.Errorf("the model list reached the backend %d times; it is a constant of this build",
			fixture.Calls())
	}
}

// And it is not an open endpoint.
func TestTheModelListNeedsTheSessionCredential(t *testing.T) {
	g := start(t)
	resp := do(t, g, request{method: http.MethodGet, path: "/v1/models", noAuth: true})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	// POST is not a model list.
	post := do(t, g, request{method: http.MethodPost, path: "/v1/models",
		headers: map[string]string{"Content-Type": "application/json"},
		body:    strings.NewReader(`{}`)})
	if post.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /v1/models = %d, want 404", post.StatusCode)
	}
}

// C3 and the boundary checks the baseline makes.
//
// Each refuses for what is actually wrong. A version this build does not speak, a body it
// cannot read, and an identifier that is not one are three different problems, and a caller
// told only "bad request" cannot act on any of them.
func TestTheRequestBoundaryIsCheckedBeforeAnythingIsRead(t *testing.T) {
	for _, c := range []struct {
		name, header, value, code string
		status                    int
	}{
		{"a version nobody here implements", "Anthropic-Version", "2024-10-22",
			"UNSUPPORTED_VERSION", http.StatusBadRequest},
		{"no version at all", "Anthropic-Version", "",
			"UNSUPPORTED_VERSION", http.StatusBadRequest},
		{"a body this build cannot decompress", "Content-Encoding", "gzip",
			"UNSUPPORTED_ENCODING", http.StatusUnsupportedMediaType},
		{"a session identifier that is not one", "X-Claude-Code-Session-Id", "has spaces",
			"INVALID_SESSION_ID", http.StatusBadRequest},
		{"an agent identifier that is not one", "X-Claude-Code-Agent-Id", "../../etc",
			"INVALID_SESSION_ID", http.StatusBadRequest},
		{"a parent identifier that is not one", "X-Claude-Code-Parent-Agent-Id",
			strings.Repeat("a", 201), "INVALID_SESSION_ID", http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]")}
			g := startWith(t, fixture)

			rq := messages(strings.NewReader(validRequest))
			if c.value == "" {
				delete(rq.headers, c.header)
			} else {
				rq.headers[c.header] = c.value
			}
			resp := do(t, g, rq)

			if resp.StatusCode != c.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, c.status, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, c.code) {
				t.Fatalf("body = %s, want %s", body, c.code)
			}
			if fixture.Calls() != 0 {
				t.Errorf("a request refused at the boundary reached the backend %d times",
					fixture.Calls())
			}
		})
	}
}

// And the shapes a real session actually sends are carried, not refused.
//
// The values here are the ones measured off the installed client: a UUID session id,
// identity encoding stated explicitly, and correlation identifiers that are absent because
// no subagent is running.
func TestTheShapesARealSessionSendsAreAccepted(t *testing.T) {
	for _, c := range []struct{ name, header, value string }{
		{"a uuid session id", "X-Claude-Code-Session-Id", "1b0b3297-ade4-4aa4-86c8-80eaf43db0a3"},
		{"an agent id beside it", "X-Claude-Code-Agent-Id", "agent_01ABCdef-ghi"},
		{"identity encoding stated", "Content-Encoding", "identity"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{
				SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")})
			rq := messages(strings.NewReader(validRequest))
			rq.headers[c.header] = c.value
			resp := do(t, g, rq)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
			}
		})
	}
}
