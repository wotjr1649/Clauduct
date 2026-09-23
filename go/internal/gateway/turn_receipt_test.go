package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func writeNativeTestFile(path string, raw []byte) error {
	if filepath.Base(filepath.Dir(path)) != "active" {
		return os.WriteFile(path, raw, 0600)
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	var receipt nativeTurnReceipt
	_ = json.Unmarshal(raw, &receipt)
	turn := receipt.Turn
	if !correlationShape.MatchString(turn) {
		turn = "fixture"
	}
	file := filepath.Join(path, strconv.Itoa(len(entries)+1)+"-"+turn)
	if err := os.WriteFile(file+".json", raw, 0600); err != nil {
		return err
	}
	return os.WriteFile(file+".ready", nil, 0600)
}

func TestIncompleteTurnPublicationNeverFallsBackAndRetryRecovers(t *testing.T) {
	g := &Gateway{}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	path := filepath.Join(dir, "active", "root")
	if err := writeNativeTestFile(path, []byte(`{"session":"s","agent":"","turn":"first"}`)); err != nil {
		t.Fatal(err)
	}
	first, found, err := g.readCurrentNativeTurn("")
	if err != nil || !found || first.Turn != "first" {
		t.Fatal("first turn missing", err)
	}
	if err := os.WriteFile(filepath.Join(path, "3-second.json"), []byte(`{"session":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := g.readCurrentNativeTurn(""); err == nil || found {
		t.Fatal("partial publication silently reused old turn")
	}
	if g.nativeEventReport().Invalid != 0 {
		t.Fatal("unpublished body parsed as committed JSON")
	}
	if err := os.WriteFile(filepath.Join(path, "3-second.json"), []byte(`{"session":"s","agent":"","turn":"second"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := g.readCurrentNativeTurn(""); err == nil || found {
		t.Fatal("body published before its completion marker")
	}
	if err := writeNativeTestFile(path, []byte(`{"session":"s","agent":"","turn":"second"}`)); err != nil {
		t.Fatal(err)
	}
	second, found, err := g.readCurrentNativeTurn("")
	if err != nil || !found || second.Turn != "second" || first.Turn != "first" {
		t.Fatal("retry or pinned receipt lost", err)
	}
}

// A long session passes the former 8192 sequence cap, and a read leaves only the
// newest publications behind so the next scan stays small.
func TestLongSessionReceiptsPassTheFormerCapAndStayPruned(t *testing.T) {
	g := &Gateway{}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	path := filepath.Join(dir, "active", "root")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	for sequence := 9000; sequence < 9000+nativePruneAbove/2+1; sequence++ {
		turn := "turn" + strconv.Itoa(sequence)
		stem := filepath.Join(path, strconv.Itoa(sequence)+"-"+turn)
		if err := os.WriteFile(stem+".json", []byte(`{"session":"s","agent":"","turn":"`+turn+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stem+".ready", nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		receipt, found, err := g.readCurrentNativeTurn("")
		if err != nil || !found || receipt.Turn != "turn9128" {
			t.Fatalf("newest receipt: found=%v err=%v turn=%q", found, err, receipt.Turn)
		}
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 2*nativeKeepRecent {
			t.Fatalf("superseded publications kept: %d entries %v", len(entries), err)
		}
	}
}

func TestMainTurnIdentityRejectsChildFieldsAndTerminalReason(t *testing.T) {
	for _, mutation := range []string{"none", "session", "agent", "turn", "model", "effort", "reason"} {
		receipt := nativeTurnReceipt{Session: "s", Turn: "turn"}
		switch mutation {
		case "session":
			receipt.Session = "other"
		case "agent":
			receipt.Agent = "child"
		case "turn":
			receipt.Turn = "../bad"
		case "model":
			receipt.Model = "gpt-5.6-sol"
		case "effort":
			receipt.Effort = "high"
		case "reason":
			receipt.Reason = "aborted"
		}
		if validActiveReceipt(receipt, "s", "") != (mutation == "none") {
			t.Fatal("main identity accepted or rejected incorrectly", mutation)
		}
	}
}

func TestIndependentRootAuxiliaryDoesNotOwnThePublishingConversationTurn(t *testing.T) {
	for _, variant := range []string{"auxiliary", "main", "subagent", "workflow", "compaction", "child", "parent", "tool", "search"} {
		t.Run(variant, func(t *testing.T) {
			g := start(t)
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			// The newer turn body exists, but publication has not completed.
			path := filepath.Join(dir, "active", "root")
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "1-new.json"), []byte(`{"session":"public","turn":"new","agent":""}`), 0600); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/v1/messages", nil)
			r.Header.Set("X-Claude-Code-Session-Id", "public")
			r.Header.Set("X-Claude-Code-Request-Class", "auxiliary")
			body := validRequest
			switch variant {
			case "main", "subagent", "workflow", "compaction":
				r.Header.Set("X-Claude-Code-Request-Class", variant)
			case "child":
				r.Header.Set("X-Claude-Code-Agent-Id", "child")
				child := filepath.Join(dir, "active", "child-child")
				if err := os.MkdirAll(child, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(child, "1-new.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent":
				r.Header.Set("X-Claude-Code-Parent-Agent-Id", "parent")
			case "tool":
				body = strings.TrimSuffix(body, "}") + `,"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`
			}
			request, err := anthropic.DecodeRequest([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			if variant == "search" {
				request.HostedSearch = &anthropic.HostedSearch{}
			}
			entry := &record{nativeTurn: &nativeTurnReceipt{Session: "stale", Turn: "stale"}}
			overrides, release, err := g.agentSelection(r, request, entry)
			release()
			if (err == nil) != (variant == "auxiliary") || entry.nativeTurn != nil || len(overrides) != 0 {
				t.Fatal("turn independence or protected selection changed", variant)
			}
		})
	}
}

// The published turn moves on after admission. The failed request still belongs to
// its admitted turn, and an end receipt for another turn cannot settle it.
func TestAReceiptThatMovedOnDoesNotRetargetAnAdmittedFailure(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{delegations: d, agents: newAgentRegistry()}
	if _, err := g.agents.register(binding, time.Now()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	old := d.results.entries[binding.ID]
	old.NativeTurn, old.NativeEndObserved, old.EndReason, old.State = "first", true, "answer", "awaiting_children"
	active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "second", Model: "gpt-5.6-luna", Effort: "high"}
	put := func(name string, receipt nativeTurnReceipt) {
		t.Helper()
		raw, _ := json.Marshal(receipt)
		if err := writeNativeTestFile(filepath.Join(dir, name), raw); err != nil {
			t.Fatal(err)
		}
	}
	name := "active/child-" + binding.ID
	put(name, active)
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-Claude-Code-Session-Id", scope.session)
	req.Header.Set("X-Claude-Code-Agent-Id", binding.ID)
	r := &record{}
	_, release, err := g.agentSelection(req, &anthropic.Request{Model: "gpt-5.6-luna"}, r)
	release()
	if err != nil || r.nativeTurn == nil || r.nativeTurn.Turn != "second" {
		t.Fatal("request did not pin its turn", err)
	}
	active.Turn = "third"
	put(name, active)
	r.data.Category = "EMPTY_REPLY"
	g.recordFailedAgentRequest(scope.session, binding.ID, r)
	current := d.results.entries[binding.ID]
	if current.NativeTurn != "second" || current.RequestFailure != "EMPTY_REPLY" || current.NativeEndObserved {
		t.Fatal("failure attached to old or later turn", current.AgentResultRecord)
	}
	put("end-"+binding.ID+"-first.json", nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Reason: "error"})
	g.reconcileNativeResults()
	if current.State != "running" || current.NativeEndObserved {
		t.Fatal("stale terminal receipt settled admitted request")
	}
	put("end-"+binding.ID+"-second.json", nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "second", Reason: "error"})
	g.reconcileNativeResults()
	parent := &anthropic.Request{}
	d.results.deliver(parent, scope.session, "")(true)
	if len(parent.Messages) != 1 || !strings.Contains(parent.Messages[0].Blocks[0].Text, "Gateway request failure: EMPTY_REPLY.") {
		t.Fatal("failure category missing from parent delivery")
	}
}

func TestFailedRequestPreservesNewerAwaitingResult(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(strconv.FormatBool(replace), func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			if _, _, err := d.route(scope, binding.ID, binding); err != nil {
				t.Fatal(err)
			}
			g := &Gateway{delegations: d, agents: newAgentRegistry()}
			if _, err := g.agents.register(binding, time.Now()); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Model: "gpt-5.6-luna", Effort: "high"}
			raw, _ := json.Marshal(active)
			if err := writeNativeTestFile(filepath.Join(dir, "active/child-"+binding.ID), raw); err != nil || !g.applyNativeTurn(binding.ID, active) {
				t.Fatal("first turn setup", err)
			}
			req := httptest.NewRequest("POST", "/v1/messages", nil)
			req.Header.Set("X-Claude-Code-Session-Id", scope.session)
			req.Header.Set("X-Claude-Code-Agent-Id", binding.ID)
			rec := &record{}
			_, release, err := g.agentSelection(req, &anthropic.Request{Model: active.Model}, rec)
			release()
			if err != nil || rec.nativeTurn == nil || rec.nativeTurn.Turn != "first" {
				t.Fatal("first request admission", err)
			}
			r := &d.results
			r.change(r.entries[binding.ID], "awaiting_children")
			if replace && !r.begin(binding.ID) {
				t.Fatal("next result refused")
			}
			current := r.entries[binding.ID]
			current.NativeTurn, current.NativeEndObserved, current.EndReason = "second", true, "answer"
			current.continuationTurn = "third"
			r.body(current, "SECOND_PUBLIC_REPORT", "native_handback")
			r.change(current, "awaiting_children")
			before, beforeBytes := current.AgentResultRecord, r.bytes
			rec.data.Category = "EMPTY_REPLY"
			g.recordFailedAgentRequest(scope.session, binding.ID, rec)
			if r.entries[binding.ID] != current || current.AgentResultRecord != before || current.body != "SECOND_PUBLIC_REPORT" || current.continuationTurn != "third" || r.bytes != beforeBytes {
				t.Fatal("late failure changed the newer result or its accounting")
			}
		})
	}
}
