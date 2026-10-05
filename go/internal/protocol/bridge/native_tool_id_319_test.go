package bridge

import "testing"

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
