package gateway

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func TestRestartKeepsOnlyTaskIdentityAndIncomingContinuation(t *testing.T) {
	for _, state := range []string{"awaiting_children", "awaiting_parent", "awaiting_native_stop"} {
		t.Run(state, func(t *testing.T) {
			r := &agentResults{entries: map[string]*agentResult{}, bytes: 3}
			old := &agentResult{AgentResultRecord: AgentResultRecord{
				Agent: "child", Session: "s", Call: "call", Selection: ResultSelection{Model: "gpt-5.6-sol", Effort: "high", Source: "agent-call"},
				State: state, NativeTurn: "old", NativeModel: "gpt-6-astra", NativeEffort: "max", NativeEndObserved: true, EndReason: "answer", RequestFailure: "OLD_FAILURE", Source: "native_transcript", Bytes: 3, Recovered: true, Review: "old_review",
			}, parent: "parent", body: "old", stopped: state == "awaiting_parent", sequence: 7, since: time.Unix(1, 0), streaming: true, stopBinding: &agentBinding{ID: "old"}, continuationTurn: "incoming"}
			r.entries["child"] = old
			if !r.begin("child") {
				t.Fatal("restart refused")
			}
			got := r.entries["child"]
			want := AgentResultRecord{Agent: old.Agent, Session: old.Session, Call: old.Call, Selection: old.Selection, Review: "not_assessed_by_gateway", State: "running"}
			if got == old || !reflect.DeepEqual(got.AgentResultRecord, want) || got.parent != "parent" || got.body != "" || got.streaming || got.stopBinding != nil || got.stopped || !got.since.After(old.since) {
				t.Fatalf("old turn state survived: %+v", got)
			}
			if (got.continuationTurn == "incoming") != (state == "awaiting_children") {
				t.Fatal("continuation authority lost or leaked")
			}
			wantBytes := 0
			if state == "awaiting_parent" {
				wantBytes = 3
			}
			if r.bytes != wantBytes {
				t.Fatal("body accounting", r.bytes)
			}
			if state != "awaiting_children" && r.entries["child/7"] == nil && state != "awaiting_native_stop" {
				t.Fatal("previous report lost")
			}
			// Fresh result with no native stop/body must not claim a transcript recovery.
			got.stopped = true
			r.change(got, "result_unavailable")
			request := &anthropic.Request{}
			r.deliver(request, "s", "parent")(true)
			if len(request.Messages) != 1 || strings.Contains(request.Messages[0].Blocks[0].Text, "One existing-result recovery") {
				t.Fatal("wrong recovery claim")
			}
			if state == "awaiting_parent" && (r.entries["child/7"] != old || old.body != "old" || strings.Contains(request.Messages[0].Blocks[0].Text, `Report: "old"`)) {
				t.Fatal("archived report lost or re-delivered as current")
			}
		})
	}
}

func TestRestartEvictionReleasesOnlyTheEvictedBody(t *testing.T) {
	r := &agentResults{}
	entry(r, "child", "child", "awaiting_parent", true, "keep")
	entry(r, "old", "old", "parent_received", true, "evict")
	for i := len(r.entries); i < maxAgents; i++ {
		id := "other" + strconv.Itoa(i)
		entry(r, id, id, "awaiting_parent", true, "")
	}
	if !r.begin("child") || len(r.entries) != maxAgents || r.entries["old"] != nil || r.bytes != len("keep") {
		t.Fatal("eviction leaked byte accounting or removed undelivered work", r.bytes, len(r.entries))
	}
}
