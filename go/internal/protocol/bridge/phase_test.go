package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
)

func TestAssistantPhaseSurvivesNativeRoundTrip(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint("deferred=", deferred), func(t *testing.T) {
			translator := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6-astra")
			if deferred {
				translator.Builder().DeferTextUntilComplete()
			}
			var reply anthropic.ResponseMessage
			for i, phase := range []string{"commentary", "final_answer", ""} {
				id := fmt.Sprintf("msg_phase_%d", i)
				item := map[string]any{"type": "message", "id": id}
				if phase != "" {
					item["phase"] = phase
				}
				opening, _ := json.Marshal(item)
				if frames, err := translator.Accept(itemAdded(i, string(opening))); err != nil || reply.Add(frames) != nil {
					t.Fatal("open", err)
				}
				var content []any
				for part, text := range []string{fmt.Sprintf("PUBLIC_PHASE_%d_A", i), fmt.Sprintf("PUBLIC_PHASE_%d_B", i)} {
					raw, _ := json.Marshal(map[string]any{"item_id": id, "content_index": part, "delta": text})
					frames, err := translator.Accept(event(codex.TextDelta, string(raw)))
					if err != nil || reply.Add(frames) != nil {
						t.Fatal("delta", err)
					}
					if !deferred && len(frames) == 0 {
						t.Fatal("text streaming was delayed")
					}
					frames, err = translator.Accept(textDoneFor(id, part, text))
					if err != nil || reply.Add(frames) != nil {
						t.Fatal("text done", err)
					}
					content = append(content, map[string]any{"type": "output_text", "text": text})
				}
				item["content"] = content
				closed, _ := json.Marshal(item)
				if frames, err := translator.Accept(itemDone(i, string(closed))); err != nil || reply.Add(frames) != nil {
					t.Fatal("close", err)
				}
			}
			frames, err := translator.Accept(event(codex.Completed, completedOK))
			if err != nil || reply.Add(frames) != nil {
				t.Fatal("complete", err)
			}
			raw, err := reply.JSON()
			if err != nil {
				t.Fatal(err)
			}
			var native struct{ Content json.RawMessage }
			if err := json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			request := decodeRequest(t, `{"model":"gpt-6-astra","max_tokens":10000,"messages":[{"role":"user","content":"PUBLIC_START"},{"role":"assistant","content":`+string(native.Content)+`},{"role":"user","content":"PUBLIC_NEXT"}]}`)
			built, err := BuildRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			var phases []string
			for _, entry := range built.Input {
				if entry.Role == "assistant" {
					phases = append(phases, entry.Phase)
				}
			}
			if !reflect.DeepEqual(phases, []string{"commentary", "final_answer", ""}) {
				t.Fatalf("phase boundaries changed: %v", phases)
			}
			encoded, err := json.Marshal(built)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct{ Input []map[string]json.RawMessage }
			if json.Unmarshal(encoded, &wire) != nil {
				t.Fatal("invalid outgoing JSON")
			}
			if _, present := wire.Input[3]["phase"]; present {
				t.Fatal("invented a phase for legacy text")
			}
			if _, err := CountInput(built); !errors.Is(err, ErrTokenCountUnsupported) {
				t.Fatal("unmeasured phase framing claimed exact token count")
			}
		})
	}
}

func TestAssistantPhaseSnapshotMismatchPreventsCompletion(t *testing.T) {
	for _, test := range []struct{ name, opening, final, completed string }{
		{"changed", `,"phase":"commentary"`, `,"phase":"final_answer"`, ""},
		{"late", "", `,"phase":"final_answer"`, ""},
		{"lost", `,"phase":"commentary"`, "", ""},
		{"completion changed", `,"phase":"commentary"`, `,"phase":"commentary"`, `,"phase":"final_answer"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			translator := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6-astra")
			opening := `{"id":"msg_phase","type":"message"` + test.opening + `}`
			final := `{"id":"msg_phase","type":"message","content":[]` + test.final + `}`
			if _, err := translator.Accept(itemAdded(0, opening)); err != nil {
				t.Fatal(err)
			}
			_, err := translator.Accept(itemDone(0, final))
			if test.completed != "" {
				if err != nil {
					t.Fatal(err)
				}
				_, err = translator.Accept(completedStating(`{"id":"msg_phase","type":"message","content":[]` + test.completed + `}`))
			}
			if !errors.Is(err, ErrItemSnapshotMismatch) {
				t.Fatalf("mismatch accepted: %v", err)
			}
		})
	}
}

func TestAssistantPhaseTrustBoundary(t *testing.T) {
	for _, phase := range []string{`"analysis"`, `""`, `42`, `{}`, `false`} {
		if _, err := codex.DecodeOutputItem([]byte(`{"output_index":0,"item":{"id":"msg_phase","type":"message","phase":`+phase+`,"content":[]}}`), true); err == nil {
			t.Fatalf("accepted backend phase %s", phase)
		}
	}
	for _, value := range []string{`"commentary"`, `"final_answer"`, `null`} {
		if _, err := codex.DecodeOutputItem([]byte(`{"output_index":0,"item":{"id":"msg_phase","type":"message","phase":`+value+`,"content":[]}}`), true); err != nil {
			t.Fatal("valid backend phase", err)
		}
	}
	for name, raw := range map[string]string{
		"user":        `{"model":"gpt-6-astra","max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":"PUBLIC","phase":"final_answer"}]}]}`,
		"system":      `{"model":"gpt-6-astra","max_tokens":100,"system":[{"type":"text","text":"PUBLIC","phase":"final_answer"}],"messages":[{"role":"user","content":"PUBLIC"}]}`,
		"tool result": `{"model":"gpt-6-astra","max_tokens":100,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_public","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_public","content":[{"type":"text","text":"PUBLIC","phase":"final_answer"}]}]}]}`,
	} {
		if _, err := anthropic.DecodeRequest([]byte(raw)); err == nil {
			t.Fatalf("accepted phase on %s text", name)
		}
		control := strings.ReplaceAll(raw, `,"phase":"final_answer"`, "")
		if _, err := anthropic.DecodeRequest([]byte(control)); err != nil {
			t.Fatalf("phase-free %s control rejected: %v", name, err)
		}
	}
	for _, value := range []string{`"analysis"`, `null`, `""`, `42`} {
		if _, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":100,"messages":[{"role":"assistant","content":[{"type":"text","text":"PUBLIC","phase":` + value + `}]}]}`)); err == nil {
			t.Fatalf("accepted native phase %s", value)
		}
	}
}

func TestExactTextCountDoesNotGuessPhaseFraming(t *testing.T) {
	built, err := BuildRequest(decodeRequest(t, `{"model":"gpt-6-luna","max_tokens":100,"messages":[{"role":"user","content":"PUBLIC_START"},{"role":"assistant","content":"PUBLIC_REPLY"},{"role":"user","content":"PUBLIC_NEXT"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CountInput(built); err != nil {
		t.Fatal("phase-free control must be countable", err)
	}
	built.Input[1].Phase = "commentary"
	if _, err := CountInput(built); !errors.Is(err, ErrTokenCountUnsupported) {
		t.Fatal("unmeasured phase framing was treated as exact", err)
	}
}
