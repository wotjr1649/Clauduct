//go:build runtime_evidence

package gateway

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Two high generations, one medium count and one medium compaction. Both
// transports enforce their route and a two-attempt ceiling before credentials.
type compactLiveTransport struct{ high, medium *upstream.Direct }

func (f compactLiveTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if call.Effort == "medium" {
		return f.medium.Execute(ctx, call)
	}
	return f.high.Execute(ctx, call)
}

func (f compactLiveTransport) Count(ctx context.Context, call upstream.Call) (int64, error) {
	return f.medium.Count(ctx, call)
}

func runtimeReplyText(body string) string {
	var text strings.Builder
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event struct{ Delta struct{ Type, Text string } }
		if json.Unmarshal([]byte(data), &event) == nil && event.Delta.Type == "text_delta" {
			text.WriteString(event.Delta.Text)
		}
	}
	return text.String()
}

// Every outbound task byte is a fixed public synthetic input below. Model output
// is checked locally, never reused as a prompt, printed or saved. Runtime/home,
// credential, TLS and destination checks are the unchanged product checks.
func TestRuntimeEvidenceAutomaticCompactionEffort(t *testing.T) {
	runtimeCompactionEffort(t, "")
}

func TestRuntimeEvidenceLongCompactionEffort(t *testing.T) {
	runtimeCompactionEffort(t, strings.Repeat("Public archived log: the sample task remained unchanged; this repeated line adds no new requirement.\n", 2048))
}

func runtimeCompactionEffort(t *testing.T, history string) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	if err := provider.CheckRuntime(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	if err := provider.CheckHome(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	const model = "gpt-5.6-luna"
	makeDirect := func(effort string) *upstream.Direct {
		d := upstream.NewDirect(provider, upstream.NewLedger(upstream.Budget{Model: model, Effort: effort, Limit: 2}), upstream.Fixed(version))
		d.Client.Timeout = 60 * time.Second
		t.Cleanup(func() { _ = d.Close() })
		return d
	}
	f := compactLiveTransport{high: makeDirect("high"), medium: makeDirect("medium")}
	defer func() {
		for _, route := range []struct {
			effort string
			direct *upstream.Direct
		}{{"high", f.high}, {"medium", f.medium}} {
			a, i, r := route.direct.Ledger.Spent()
			t.Logf("model=%s effort=%s attempts=%d inferences=%d refused=%d", model, route.effort, a, i, r)
		}
	}()
	g := startWith(t, f)
	g.EnableContextPolicy()
	checkGeneration := func(label, prompt, class, effort string, markers ...string) {
		t.Helper()
		status, body := effortRequest(t, g, model, "high", prompt, class, false)
		waitForActive(t, g, 0, label)
		recent := g.Snapshot().Recent
		last := recent[len(recent)-1]
		retained, text := true, runtimeReplyText(body)
		for _, marker := range markers {
			retained = retained && strings.Contains(text, marker)
		}
		t.Logf("step=%s status=%d category=%s model=%s effort=%s marker_retained=%v", label, status, last.Category, last.Model, last.Effort, retained)
		if status != 200 || last.Category != "" || last.Model != model || last.Effort != effort || !retained || last.InputTokens == nil || last.OutputTokens == nil || last.UsageSource != "backend" {
			t.Fatal("live generation evidence incomplete")
		}
		t.Logf("step=%s input_tokens=%d output_tokens=%d count_source=%s count_agreement=%s", label, *last.InputTokens, *last.OutputTokens, last.CountSource, last.CountAgreement)
	}
	checkGeneration("before", "Public synthetic test. Reply exactly PUBLIC_BEFORE.", "main", "high", "PUBLIC_BEFORE")
	ticket := triggeredCompactEvent(t, g, "auto")
	prompt := strings.Replace(compactPrompt(), "synthetic history.", "synthetic history.\n"+history+"\nRequired retained facts: decision PUBLIC_DECISION_17; constraint PUBLIC_LIMIT_3; failed check PUBLIC_FAIL_2; next action PUBLIC_NEXT_9. Preserve all four exact identifiers in the summary.", 1) + ticket
	status, _ := effortRequest(t, g, model, "high", prompt, "compaction", true)
	if status != 200 {
		t.Fatalf("live count status=%d", status)
	}
	checkGeneration("compact", prompt, "compaction", "medium", "PUBLIC_DECISION_17", "PUBLIC_LIMIT_3", "PUBLIC_FAIL_2", "PUBLIC_NEXT_9")
	recent := g.Snapshot().Recent
	last := recent[len(recent)-1]
	if last.CountSource != "prior-count-cache" || last.CountAgreement != "matched" {
		t.Fatal("live compaction count did not match usage")
	}
	checkGeneration("after", "Public synthetic followup. Reply exactly PUBLIC_AFTER.", "main", "high", "PUBLIC_AFTER")
	for _, direct := range []*upstream.Direct{f.high, f.medium} {
		attempts, _, refused := direct.Ledger.Spent()
		if attempts != 2 || refused != 0 {
			t.Fatal("unexpected live attempt count")
		}
	}
}
