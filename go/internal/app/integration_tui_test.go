//go:build runtime_evidence

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Session counters survive eviction. Compaction is not a normal answer, and an
// auxiliary refusal is not a cancelled generation. Keep budget refusals visible.
func assessTUIDiagnostics(d gateway.Diagnostics) error {
	var generation, compaction gateway.FeatureEvidence
	for _, feature := range d.Features {
		switch feature.Name {
		case "generation":
			generation = feature
		case "compaction":
			compaction = feature
		}
	}
	if generation.Completed != 3 || generation.Cancelled != 1 || compaction.Completed != 1 || compaction.Failed != 0 || compaction.Cancelled != 0 {
		return errors.New("TUI_REQUEST_SEQUENCE_INCOMPLETE")
	}
	for category, count := range d.Totals.Failures {
		if count > 0 && category != "CANCELLED" && category != "REQUEST_BUDGET" && category != "ROUTE_NOT_AUTHORISED" {
			return errors.New("TUI_UNEXPECTED_FAILURE")
		}
	}
	if d.Totals.Failures["CANCELLED"] != 1 {
		return errors.New("TUI_CANCELLATION_INCOMPLETE")
	}
	return nil
}

// Only the task-owned temporary profile is read. Root confines native paths;
// limits and fixed errors prevent arbitrary transcript content escaping the test.
func assessTUIProject(profile string) error {
	root, err := os.OpenRoot(profile)
	if err != nil {
		return errors.New("TUI_PROFILE_UNREADABLE")
	}
	defer root.Close()
	paths, err := fs.Glob(root.FS(), "projects/*/*.jsonl")
	if err != nil || len(paths) != 1 {
		return errors.New("TUI_TRANSCRIPT_AMBIGUOUS")
	}
	f, err := root.Open(paths[0])
	if err != nil {
		return errors.New("TUI_TRANSCRIPT_UNREADABLE")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return errors.New("TUI_TRANSCRIPT_LIMIT")
	}
	return assessTUITranscript(f, strings.TrimSuffix(filepath.Base(paths[0]), ".jsonl"))
}

func assessTUITranscript(input io.Reader, session string) error {
	scanner := bufio.NewScanner(io.LimitReader(input, (2<<20)+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	stage, size := 0, 0
	for scanner.Scan() {
		size += len(scanner.Bytes()) + 1
		if size > 2<<20 {
			return errors.New("TUI_TRANSCRIPT_LIMIT")
		}
		var row struct {
			Type, Subtype, SessionID string
			Message                  struct{ Content json.RawMessage }
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			return errors.New("TUI_TRANSCRIPT_INVALID")
		}
		if row.Subtype == "compact_boundary" {
			if stage != 1 || row.SessionID != session {
				return errors.New("TUI_COMPACTION_ORDER")
			}
			stage = 2
			continue
		}
		if row.Type != "assistant" && row.Type != "user" {
			continue
		}
		if row.SessionID != session {
			return errors.New("TUI_SESSION_MISMATCH")
		}
		var text string
		if json.Unmarshal(row.Message.Content, &text) != nil {
			var blocks []struct{ Type, Text string }
			if json.Unmarshal(row.Message.Content, &blocks) != nil {
				return errors.New("TUI_CONTENT_INVALID")
			}
			for _, block := range blocks {
				if block.Type == "text" {
					text += block.Text
				}
			}
		}
		text = strings.Join(strings.Fields(text), " ")
		if row.Type == "user" {
			if strings.HasPrefix(text, "[Request interrupted by user") {
				if stage != 3 {
					return errors.New("TUI_CANCELLATION_ORDER")
				}
				stage = 4
			}
			continue
		}
		switch text {
		case "PUBLIC_TUI_47":
			if stage != 0 {
				return errors.New("TUI_INITIAL_ORDER")
			}
			stage = 1
		case "PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE":
			if stage != 2 {
				return errors.New("TUI_FACTS_ORDER")
			}
			stage = 3
		case "PUBLIC_TUI_RECOVERED":
			if stage != 4 {
				return errors.New("TUI_RECOVERY_ORDER")
			}
			stage = 5
		}
	}
	if scanner.Err() != nil || stage != 5 {
		return errors.New("TUI_TRANSCRIPT_INCOMPLETE")
	}
	return nil
}

func TestTUIAcceptanceRejectsMissingOrMisorderedEvidence(t *testing.T) {
	rows := []string{
		`{"type":"assistant","sessionId":"public","message":{"content":[{"type":"text","text":"PUBLIC_TUI_47"}]}}`,
		`{"type":"system","subtype":"compact_boundary","sessionId":"public"}`,
		`{"type":"assistant","sessionId":"public","message":{"content":[{"type":"text","text":"PUBLIC_TUI_47\nblue\n47\nPUBLIC_COMPACT_DONE"}]}}`,
		`{"type":"user","sessionId":"public","message":{"content":"[Request interrupted by user]"}}`,
		`{"type":"assistant","sessionId":"public","message":{"content":[{"type":"text","text":"PUBLIC_TUI_RECOVERED"}]}}`,
	}
	for _, mutation := range []string{"none", "missing_initial", "missing_compact", "wrong_fact", "missing_cancel", "missing_recovery", "early_recovery", "user_echo", "other_session", "malformed", "oversized", "recent_evicted", "compact_as_answer", "no_cancel", "totals_cancel_zero", "unexpected_failure"} {
		t.Run(mutation, func(t *testing.T) {
			copyRows := append([]string(nil), rows...)
			d := gateway.Diagnostics{Features: []gateway.FeatureEvidence{{Name: "generation", Completed: 3, Cancelled: 1}, {Name: "compaction", Completed: 1}}, Totals: gateway.SessionTotals{Failures: map[string]int64{"CANCELLED": 1}}}
			switch mutation {
			case "missing_initial":
				copyRows[0] = `{}`
			case "missing_compact":
				copyRows[1] = `{}`
			case "wrong_fact":
				copyRows[2] = strings.Replace(copyRows[2], "blue", "red", 1)
			case "missing_cancel":
				copyRows[3] = `{}`
			case "missing_recovery":
				copyRows[4] = `{}`
			case "early_recovery":
				copyRows[3], copyRows[4] = copyRows[4], copyRows[3]
			case "user_echo":
				copyRows[2] = strings.Replace(copyRows[2], "assistant", "user", 1)
			case "other_session":
				copyRows[4] = strings.Replace(copyRows[4], `"public"`, `"other"`, 1)
			case "malformed":
				copyRows = append(copyRows, `{`)
			case "oversized":
				copyRows = append(copyRows, strings.Repeat(" ", 2<<20))
			case "recent_evicted":
				d.Recent = make([]gateway.RequestRecord, 16)
			case "compact_as_answer":
				d.Features[0].Completed = 2
			case "no_cancel":
				d.Features[0].Cancelled = 0
			case "totals_cancel_zero":
				d.Totals.Failures["CANCELLED"] = 0
			case "unexpected_failure":
				d.Totals.Failures["UPSTREAM_FAILURE"] = 1
			}
			passed := assessTUIDiagnostics(d) == nil && assessTUITranscript(strings.NewReader(strings.Join(copyRows, "\n")), "public") == nil
			if passed != (mutation == "none" || mutation == "recent_evicted") {
				t.Fatalf("incorrect acceptance: mutation=%s passed=%v", mutation, passed)
			}
		})
	}
}

// This local run tests the native client, gateway and acceptance evaluator, not
// backend model quality. It has no Direct transport, credential provider or dial.
func TestRuntimeEvidenceNativeTUILocal(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_TUI_LOCAL") != "1" {
		t.Skip("local PTY switch absent")
	}
	env, cwd := integrationWorkspace(t)
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 5})
	runIntegrationTUI(t, &tuiLocalTransport{ledger}, ledger, env, cwd)
	t.Log("local synthetic transport only: external_requests=0 credential_reads=0")
}

type tuiLocalTransport struct{ ledger *upstream.Ledger }

func (d *tuiLocalTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if err := d.ledger.Reserve(upstream.Attempt{Requested: call.Requested, Model: call.Model, Effort: call.Effort, Source: call.Source}); err != nil {
		return nil, err
	}
	var request struct {
		Input []struct {
			Role, Type string
			Content    json.RawMessage
		}
	}
	if json.Unmarshal(call.Body, &request) != nil {
		return nil, errors.New("LOCAL_TUI_INPUT")
	}
	last := ""
	for _, entry := range request.Input {
		if entry.Role == "user" {
			var parts []struct{ Text string }
			if json.Unmarshal(entry.Content, &parts) != nil {
				return nil, errors.New("LOCAL_TUI_USER_INPUT")
			}
			last = ""
			for _, part := range parts {
				last += part.Text
			}
		}
	}
	answer := "PUBLIC_TUI_47"
	switch {
	case strings.Contains(string(call.Body), "CRITICAL: Respond with TEXT ONLY"):
		answer = "<summary>Retain PUBLIC_TUI_47, blue, and release 47.</summary>"
	case strings.Contains(last, "PUBLIC_TUI_RECOVERED"):
		answer = "PUBLIC_TUI_RECOVERED"
	case strings.Contains(last, "PUBLIC_CANCEL_STARTED"):
		partial, _, _ := strings.Cut(textStream("public_cancel", "PUBLIC_CANCEL_STARTED"), `data: {"type":"response.output_text.done"`)
		return &upstream.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(partial), tuiAwaitCancellation{ctx}))}, nil
	case strings.Contains(last, "PUBLIC_COMPACT_DONE"):
		answer = "PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE"
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(textStream("public_tui", answer), call, 100)}).Execute(ctx, call)
}

type tuiAwaitCancellation struct{ context.Context }

func (r tuiAwaitCancellation) Read([]byte) (int, error) {
	<-r.Done()
	return 0, r.Err()
}
