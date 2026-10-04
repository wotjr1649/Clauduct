package bridge

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// #214: native sees a step-marked id, and the backend gets its own call_id back.
func TestNativeToolIDRoundTripsToTheBackendCallID(t *testing.T) {
	id, err := NativeToolID("call_abc123", "0123456789ab")
	if err != nil || id != "call_abc123__cdt0123456789ab" {
		t.Fatalf("NativeToolID = %q, %v", id, err)
	}
	if got := BackendCallID(id); got != "call_abc123" {
		t.Fatalf("BackendCallID(%q) = %q", id, got)
	}
	// Earlier builds' conversations carry unmarked ids; they pass unchanged.
	for _, unmarked := range []string{"call_abc123", "toolu_plugin_f4786e0d", "x__cdt0123", "x__cdt0123456789AB"} {
		if got := BackendCallID(unmarked); got != unmarked {
			t.Fatalf("BackendCallID(%q) = %q", unmarked, got)
		}
	}
	// A call_id that already looks marked could not be restored after marking.
	if _, err := NativeToolID("call__cdt0123456789ab", "0123456789ab"); !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
		t.Fatalf("pre-marked call_id: %v", err)
	}
	for _, tag := range []string{"", "0123456789a", "0123456789abc", "0123456789AB"} {
		if _, err := NativeToolID("call_abc123", tag); !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
			t.Fatalf("tag %q: %v", tag, err)
		}
	}
}

func TestBackendRequestCarriesUnmarkedCallIDs(t *testing.T) {
	request := decodeRequest(t, `{"model":"gpt-6-sol","max_tokens":64,"stream":true,
		"tools":[{"name":"Read","input_schema":{"type":"object"}}],
		"messages":[{"role":"user","content":"read"},
		{"role":"assistant","content":[{"type":"tool_use","id":"call_abc123__cdt0123456789ab","name":"Read","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_abc123__cdt0123456789ab","content":"public"}]}]}`)
	built, err := BuildRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, entry := range built.Input {
		if entry.Type == "function_call" || entry.Type == "function_call_output" {
			calls++
			if entry.CallID != "call_abc123" {
				t.Fatalf("%s call_id = %q", entry.Type, entry.CallID)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestTranslatorNamesCallsForNativeBeforePreparation(t *testing.T) {
	translator := NewTranslatorFor(callable("Read"), "")
	translator.ToolUseID = func(callID string) (string, error) { return NativeToolID(callID, "0123456789ab") }
	prepared := ""
	translator.PrepareToolCall = func(id, _ string, raw json.RawMessage) (json.RawMessage, error) {
		prepared = id
		return raw, nil
	}
	var frames []anthropic.Frame
	for _, e := range flatten(t, []any{completedWith(functionCall("call_abc123", "Read", `{}`))}) {
		produced, err := translator.Accept(e)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, produced...)
	}
	block := blockAt(t, frames, "content_block_start", 0)["content_block"].(map[string]any)
	if block["id"] != "call_abc123__cdt0123456789ab" || prepared != block["id"] {
		t.Fatalf("native id = %v, prepared = %q", block["id"], prepared)
	}

	refused := NewTranslatorFor(callable("Read"), "")
	refused.ToolUseID = func(string) (string, error) { return "", anthropic.ErrUnsupportedToolCall }
	var failure error
	for _, e := range flatten(t, []any{completedWith(functionCall("call_abc123", "Read", `{}`))}) {
		if _, failure = refused.Accept(e); failure != nil {
			break
		}
	}
	if !errors.Is(failure, anthropic.ErrUnsupportedToolCall) {
		t.Fatalf("unnamed call delivered: %v", failure)
	}
}
