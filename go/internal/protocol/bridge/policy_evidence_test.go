//go:build policy_evidence

package bridge

import (
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"testing"
)

func TestPolicyEvidenceCompactionPreservesResolvedEffort(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			req := &anthropic.Request{Model: "gpt-5.6-luna", Effort: effort, Messages: []anthropic.Message{text("user", compaction("synthetic transcript"))}}
			got, err := BuildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("resolved=%s/%s delivered=%s/%s", req.Model, effort, got.Model, got.Effort.Effort)
			if got.Model != req.Model || got.Effort.Effort != effort {
				t.Error("compaction changed the selection")
			}
		})
	}
}
