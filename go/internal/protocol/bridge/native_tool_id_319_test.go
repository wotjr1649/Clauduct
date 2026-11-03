package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"strings"
	"testing"
)

func TestOpaqueHistoryIDsAreNotGuessed319(t *testing.T) {
	r := decodeRequest(t, `{"model":"gpt-6-sol","max_tokens":64,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"plain","name":"Read","input":{}},{"type":"tool_use","id":"plain__cdt0123456789ab","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"plain","content":"A"},{"type":"tool_result","tool_use_id":"plain__cdt0123456789ab","content":"B"}]}]}`)
	built, err := BuildRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range built.Input {
		if e.Type == "function_call" || e.Type == "function_call_output" {
			got = append(got, e.CallID)
		}
	}
	want := []string{"plain", "plain__cdt0123456789ab", "plain", "plain__cdt0123456789ab"}
	if len(got) != len(want) {
		t.Fatal("pair count")
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("opaque identity lost at %d", i)
		}
	}
}

func TestRawDuplicateRejectedBeforeMapping319(t *testing.T) {
	tr := NewTranslatorFor(callable("Read"), "")
	reserved, prepared := 0, 0
	tr.ToolUseID = func(string) (string, error) { reserved++; return fmt.Sprintf("fresh_%d", reserved), nil }
	tr.PrepareToolCall = func(_ string, _ string, raw json.RawMessage) (json.RawMessage, error) { prepared++; return raw, nil }
	events := completedWith(functionItem("item_a", "raw_same", "Read", `{}`), functionItem("item_b", "raw_same", "Read", `{}`))
	for i, e := range events {
		frames, err := tr.Accept(e)
		if i < len(events)-1 {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if !errors.Is(err, anthropic.ErrUnsupportedToolCall) || reserved != 0 || prepared != 0 || len(frames) != 0 {
			t.Fatalf("duplicate reached mapping: err=%v reserved=%d prepared=%d frames=%d", err, reserved, prepared, len(frames))
		}
	}
}

func TestRawIdentityCannotBeLaunderedByMapping319(t *testing.T) {
	for _, c := range []struct {
		id    string
		valid bool
	}{{"", false}, {"space id", false}, {strings.Repeat("x", 201), false}, {"plain__cdt0123456789ab", true}, {strings.Repeat("x", 200), true}} {
		id := c.id
		t.Run(fmt.Sprintf("length-%d", len(id)), func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			reserved := 0
			tr.ToolUseID = func(string) (string, error) { reserved++; return "valid_native", nil }
			var got error
			for _, e := range completedWith(functionItem("item", id, "Read", `{}`)) {
				if _, got = tr.Accept(e); got != nil {
					break
				}
			}
			valid := c.valid
			if valid && (got != nil || reserved != 1) {
				t.Fatalf("valid opaque ID rejected: err=%v reserved=%d", got, reserved)
			}
			if !valid && (got == nil || reserved != 0) {
				t.Fatalf("raw identity laundered: err=%v reserved=%d", got, reserved)
			}
		})
	}
}
