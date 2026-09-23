package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Real native SDK, gateway, child and receipts; only backend bytes are synthetic.
// Closing the parent's empty terminal reply releases the child, fixing the race
// without sleeps or a model deciding when to finish.
func TestNativeSDKEmptyReplyWaitsForBackgroundChild(t *testing.T) {
	buildHook(t)
	for _, tc := range []struct {
		name                 string
		workflow, emptyFinal bool
	}{{"Agent", false, false}, {"Workflow", true, false}, {"empty notification", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &nativeWaitTransport{childStarted: make(chan struct{}), releaseChild: make(chan struct{}), workflow: tc.workflow, emptyFinal: tc.emptyFinal}
			const roles = `{"public-wait":{"description":"Public worker","prompt":"Use no tools.","model":"gpt-5.6-sol","effort":"low","tools":[]}}`
			out := (nativeRun{Args: []string{"-p", "Start one public worker and acquire its result.", "--model", "gpt-5.6-luna", "--effort", "low", "--allowedTools", "Agent,Workflow,ToolSearch", "--agents", roles, "--output-format", "stream-json", "--verbose"}, ContextPolicy: true, transport: transport}).run(t)
			defer func() {
				if t.Failed() {
					for _, r := range out.result.Diagnostics.Recent {
						if r.ParentReadiness != nil {
							t.Logf("class=%s status=%d category=%s wait=%+v", r.RequestClass, r.Status, r.Category, r.ParentReadiness)
						}
					}
					t.Logf("native output=%s", tail(out.output(), 1500))
				}
			}()
			if out.err != nil || out.result.NativeExitCode != 0 || !tc.emptyFinal && !strings.Contains(out.stdout, "PUBLIC_WAIT_DONE") {
				t.Fatalf("SDK parent did not recover: exit=%d failures=%v root=%d child=%d", out.result.NativeExitCode, out.result.Diagnostics.Totals.Failures, transport.parents.Load(), transport.children.Load())
			}
			parents := int32(3)
			if tc.workflow {
				parents++
			}
			if transport.parents.Load() != parents || transport.children.Load() != 1 || out.result.Diagnostics.AgentResults.Totals["parent_received"] != 1 {
				t.Fatalf("SDK wait repeated work or lost child: root=%d child=%d received=%d", transport.parents.Load(), transport.children.Load(), out.result.Diagnostics.AgentResults.Totals["parent_received"])
			}
			if len(out.result.Diagnostics.Totals.Failures) != 0 {
				t.Fatal("SDK wait hid an error", out.result.Diagnostics.Totals.Failures)
			}
			for _, line := range strings.Split(out.stdout, "\n") {
				var event struct {
					Type    string
					IsError bool `json:"is_error"`
				}
				if json.Unmarshal([]byte(line), &event) == nil && event.Type == "result" && event.IsError {
					t.Fatal("native reported an intermediate execution error")
				}
			}
			completed := int64(1)
			if tc.emptyFinal {
				completed++
			}
			for _, feature := range out.result.Diagnostics.Features {
				if (feature.Name == "parent_wait" || feature.Name == "empty_reply_wait") && (feature.Completed != completed || feature.Unconfirmed != 0) {
					t.Fatal("SDK wait lost verified feature evidence", feature)
				}
			}
			for _, record := range out.result.Diagnostics.Recent {
				if ready := record.ParentReadiness; ready != nil && ready.Withheld && ready.Empty && ready.ControlMode == "sdk" {
					return
				}
			}
			t.Fatal("SDK empty waiting reply was not observed")
		})
	}
}

type nativeWaitTransport struct {
	parents, children          atomic.Int32
	childStarted, releaseChild chan struct{}
	workflow, emptyFinal       bool
}

func (p *nativeWaitTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if call.Model == "gpt-5.6-sol" {
		if p.children.Add(1) != 1 {
			return nil, errors.New("PUBLIC_DUPLICATE_CHILD")
		}
		close(p.childStarted)
		select {
		case <-p.releaseChild:
			return (&upstream.Fixture{SSE: textStream("public_child", "PUBLIC_WAIT_CHILD")}).Execute(ctx, call)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	index := p.parents.Add(1)
	if p.workflow {
		if index == 1 {
			return (&upstream.Fixture{SSE: toolStream("public_discover", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)}).Execute(ctx, call)
		}
		index--
	}
	switch index {
	case 1:
		if p.workflow {
			return (&upstream.Fixture{SSE: toolStream("public_workflow", "Workflow", `{"script":"clauduct:plan-v1","args":{"steps":[{"id":"W","prompt":"Reply PUBLIC_WAIT_CHILD","model":"gpt-5.6-sol","effort":"low","tools":[]}]}}`)}).Execute(ctx, call)
		}
		return (&upstream.Fixture{SSE: toolStream("public_wait_call", "Agent", `{"subagent_type":"public-wait","description":"Public child","prompt":"Reply PUBLIC_WAIT_CHILD","model":"gpt-5.6-sol","effort":"low","run_in_background":true}`)}).Execute(ctx, call)
	case 2:
		select {
		case <-p.childStarted:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		empty := frames(`{"type":"response.created","response":{"id":"public_wait"}}`, `{"type":"response.completed","response":{"id":"public_wait","usage":{"input_tokens":9,"output_tokens":0},"output":[]}}`)
		return &upstream.Response{Body: &nativeWaitBody{Reader: strings.NewReader(empty), release: p.releaseChild}}, nil
	case 3:
		if p.emptyFinal {
			return (&upstream.Fixture{SSE: frames(`{"type":"response.created","response":{"id":"public_notification"}}`, `{"type":"response.completed","response":{"id":"public_notification","usage":{"input_tokens":9,"output_tokens":0},"output":[]}}`)}).Execute(ctx, call)
		}
		return (&upstream.Fixture{SSE: textStream("public_parent", "PUBLIC_WAIT_CHILD PUBLIC_WAIT_DONE")}).Execute(ctx, call)
	default:
		return nil, errors.New("PUBLIC_DUPLICATE_PARENT")
	}
}

type nativeWaitBody struct {
	io.Reader
	once    sync.Once
	release chan struct{}
}

func (b *nativeWaitBody) Close() error {
	b.once.Do(func() { close(b.release) })
	return nil
}
