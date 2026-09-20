package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type deadlineProcess struct {
	stopped       chan struct{}
	once          sync.Once
	cancel        context.CancelFunc
	requestExited chan struct{}
}

// Model the real process contract: Stop ends the child-owned HTTP request and
// Wait cannot report a reaped child while that request is still running.
func (p *deadlineProcess) Wait() error { <-p.stopped; <-p.requestExited; return nil }
func (p *deadlineProcess) Stop() error {
	p.once.Do(func() { p.cancel(); close(p.stopped) })
	return nil
}
func (p *deadlineProcess) ExitCode() int { return 1 }

type drainingTransport struct {
	upstream.Fixture
	entered, release chan struct{}
}

func (f *drainingTransport) Execute(ctx context.Context, c upstream.Call) (*upstream.Response, error) {
	close(f.entered)
	select {
	case <-f.release:
		return f.Fixture.Execute(ctx, c)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestSessionDeadlineDrainsBeforeStoppingAndBoundsStalledWork(t *testing.T) {
	for _, stalled := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed_turn", true: "grace_exhausted"}[stalled], func(t *testing.T) {
			f := &drainingTransport{Fixture: upstream.Fixture{SSE: measuredStream("public")}, entered: make(chan struct{}), release: make(chan struct{})}
			clientCtx, cancelClient := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelClient()
			p := &deadlineProcess{stopped: make(chan struct{}), cancel: cancelClient, requestExited: make(chan struct{})}
			var gw *gateway.Gateway
			requestDone := make(chan error, 1)
			var phase atomic.Int32
			started := time.Now()
			var traceMu sync.Mutex
			var trace []string
			note := func(event string) {
				traceMu.Lock()
				defer traceMu.Unlock()
				trace = append(trace, fmt.Sprintf("%d:%s", time.Since(started).Milliseconds(), event))
			}
			defer func() {
				traceMu.Lock()
				defer traceMu.Unlock()
				t.Logf("deadline trace %v", trace)
			}()
			grace := 10 * time.Second
			if stalled {
				grace = 100 * time.Millisecond
			}
			var draining, stopping bool
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			r, err := Run(ctx, Options{Settings: &noSettings, Env: map[string]string{}, Cwd: t.TempDir(), Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
				SessionTimeout: 100 * time.Millisecond, DeadlineGrace: grace,
				ResolveClaude: func() (string, bool, error) { return "synthetic", true, nil },
				StartGateway:  func() (*gateway.Gateway, error) { var e error; gw, e = gateway.Start(f); return gw, e },
				StartProcess: func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error) {
					go func() {
						defer close(p.requestExited)
						phase.Store(1)
						note("client_start")
						req, _ := http.NewRequestWithContext(clientCtx, "POST", gw.BaseURL()+"/v1/messages", strings.NewReader(`{"model":"gpt-6-astra","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"public"}]}`))
						req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
							GotConn:              func(httptrace.GotConnInfo) { note("client_connected") },
							WroteRequest:         func(httptrace.WroteRequestInfo) { note("client_sent") },
							GotFirstResponseByte: func() { note("client_first_byte") },
						}))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Anthropic-Version", "2023-06-01")
						req.Header.Set("Authorization", "Bearer "+gw.Token())
						resp, e := http.DefaultClient.Do(req)
						phase.Store(2)
						note("client_headers_returned")
						if e == nil {
							if resp.StatusCode != 200 {
								body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
								e = fmt.Errorf("synthetic request refused: %d %s", resp.StatusCode, body)
							} else {
								_, e = io.Copy(io.Discard, resp.Body)
							}
							resp.Body.Close()
						}
						phase.Store(3)
						note("client_body_closed")
						requestDone <- e
						phase.Store(4)
					}()
					select {
					case <-f.entered:
						note("upstream_entered")
						return p, nil
					case e := <-requestDone:
						return nil, e
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
				Checkpoint: func(s Status) error {
					note("checkpoint_" + s.Lifecycle.State)
					if s.Lifecycle.Final || s.ExitCode != ExitCodeUnknown {
						t.Error("checkpoint claimed terminal success")
					}
					if s.Lifecycle.State == "draining" && !draining {
						draining = true
						if s.Gateway.Requests.Active != 1 {
							t.Error("lost active request at deadline")
						}
						select {
						case <-p.stopped:
							t.Error("deadline killed active turn")
						default:
						}
						if !stalled {
							close(f.release)
						}
					}
					if s.Lifecycle.State == "stopping" {
						stopping = true
					}
					if stalled {
						return errors.New("synthetic disk unavailable")
					}
					return nil
				},
			})
			runElapsed := time.Since(started)
			note("run_returned")
			if err != nil || r.Category != CategoryDeadline || !r.Lifecycle.Final || !r.Lifecycle.NativeReaped || !draining || !stopping || r.Lifecycle.GraceExpired != stalled {
				t.Fatalf("deadline result: %v %+v", err, r.Lifecycle)
			}
			if r.Diagnostics.Requests.Active != 0 || r.CleanupErr != nil {
				t.Fatal("final snapshot preceded gateway drain")
			}
			if stalled && r.Lifecycle.CheckpointFailures == 0 {
				t.Fatal("checkpoint failure was hidden")
			}
			select {
			case <-p.requestExited:
			default:
				t.Fatal("reaped child still owns a running HTTP request")
			}
			select {
			case e := <-requestDone:
				if !stalled && e != nil {
					t.Fatalf("completed request lost: %v", e)
				}
			case <-ctx.Done():
				t.Fatalf("request leaked: clientPhase=%d runMs=%d finishedMs=%d totalMs=%d", phase.Load(), runElapsed.Milliseconds(), r.Lifecycle.ObservedAt.Sub(started).Milliseconds(), time.Since(started).Milliseconds())
			}
		})
	}
}

func TestCheckpointAtomicReplacementAndInvalidLimits(t *testing.T) {
	for _, raw := range []string{"0", "-1", "NaN", "100ms", "43200001", "9223372036854775807"} {
		if _, err := SessionDuration(map[string]string{"CLAUDUCT_SESSION_TIMEOUT_MS": raw}); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if d, err := SessionDuration(nil); err != nil || d != 0 {
		t.Fatal("ordinary session acquired a deadline")
	}
	if d, err := SessionDuration(map[string]string{"CLAUDUCT_SESSION_TIMEOUT_MS": "1800000"}); err != nil || d != 30*time.Minute {
		t.Fatal("wrong explicit duration")
	}
	t.Setenv("TMP", t.TempDir())
	t.Setenv("TEMP", os.Getenv("TMP"))
	file, err := writeStatus([]byte(`{"phase":"running"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writeStatus([]byte(`{"phase":"draining"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = writeStatus(make([]byte, (8<<20)+1)); err == nil {
		t.Fatal("accepted unbounded status")
	}
	raw, err := os.ReadFile(file)
	var got map[string]string
	if err != nil || json.Unmarshal(raw, &got) != nil || got["phase"] != "draining" {
		t.Fatal("failed checkpoint destroyed prior evidence")
	}
	files, _ := os.ReadDir(StatusDir())
	if len(files) != 1 {
		t.Fatal("temporary files left behind")
	}
}

func TestDrainWaitsAcrossCompactionRecountGap(t *testing.T) {
	for _, phase := range []string{"required", "compacting", "recount"} {
		if drainReady(gateway.Diagnostics{AgentContexts: []gateway.AgentContextReport{{Phase: phase}}}) {
			t.Fatalf("stopped during %s", phase)
		}
	}
	if !drainReady(gateway.Diagnostics{AgentContexts: []gateway.AgentContextReport{{Phase: "ready"}}}) {
		t.Fatal("finished compaction blocked exit")
	}
}

func TestDrainDoesNotMistakeMissingHTTPForFinishedNativeWork(t *testing.T) {
	for _, phase := range []string{"request", "tool_pending", "progress_unconfirmed"} {
		if drainReady(gateway.Diagnostics{Progress: gateway.NativeProgressReport{Agents: []gateway.NativeProgress{{Phase: phase}}}}) {
			t.Fatalf("drained native work during %s", phase)
		}
	}
	if drainReady(gateway.Diagnostics{Progress: gateway.NativeProgressReport{Unreadable: 1}}) {
		t.Fatal("unreadable progress treated as idle")
	}
	if drainReady(gateway.Diagnostics{AgentResults: gateway.AgentResultReport{Current: map[string]int64{"awaiting_children": 1}}}) {
		t.Fatal("waiting parent treated as idle")
	}
	if !drainReady(gateway.Diagnostics{Progress: gateway.NativeProgressReport{Agents: []gateway.NativeProgress{{Phase: "turn_ended"}}}}) {
		t.Fatal("ended work never drains")
	}
}

func BenchmarkCrashCheckpoint(b *testing.B) {
	b.Setenv("TMP", b.TempDir())
	b.Setenv("TEMP", os.Getenv("TMP"))
	account := Status{Category: "RUNNING", ExitCode: ExitCodeUnknown, Lifecycle: &LifecycleFacts{State: "running"}}
	for i := 0; i < 16; i++ {
		account.Gateway.Recent = append(account.Gateway.Recent, gateway.RequestRecord{Seq: int64(i), Model: "gpt-6-astra", Effort: "low", Kind: "generation", Stage: "upstream", Outcome: "in-progress"})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := WriteCheckpoint(account); err != nil {
			b.Fatal(err)
		}
	}
}
