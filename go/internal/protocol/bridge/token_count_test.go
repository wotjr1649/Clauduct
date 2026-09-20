package bridge

import (
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"strings"
	"testing"
)

func TestNewRouteDoesNotInheritCountValidation(t *testing.T) {
	prior := Models
	Models = append(append([]Model(nil), prior...), Model{Key: "future", ID: "future-public-fixture", Effort: "low", Context: ContextPolicy{500000, 450000}})
	t.Cleanup(func() { Models = prior })
	request := &Request{Model: "future-public-fixture", Instruction: Instruction, Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: "Reply OK."}}}}}
	if _, err := SelectRoute(request.Model, "low"); err != nil {
		t.Fatal("fixture route unavailable")
	}
	if _, err := CountInput(request); !errors.Is(err, ErrTokenCountUnsupported) || BackendCountSupported(request) {
		t.Fatal("routing enabled unvalidated token counting")
	}
}

func TestLocalTextCounterMatchesRecordedBackendCounts(t *testing.T) {
	// Expected values are backend response.completed usage, not tokenizer output.
	cases := []struct {
		name, text, system string
		want               int64
	}{
		{"short", "Reply OK.", "", 21},
		{"english", "Public synthetic token test: the quick brown fox jumps over the lazy dog. Reply OK.", "", 36},
		{"korean", "공개 합성 테스트입니다. 모델별 입력 토큰 수를 확인합니다. OK만 답하세요.", "", 40},
		{"unicode", "Public sample: café naïve 東京 😀. Reply OK.", "", 30},
		{"code", "Public code sample: func add(a, b int) int { return a + b }\nReply OK.", "", 39},
		{"system", "Reply OK.", "This is a public synthetic tokenizer test. Answer briefly.", 36},
		{"space", "  Reply OK.  ", "", 23},
		{"lines", "\n\nReply\tOK.\r\n", "", 23},
		{"contractions", "We're testing; don't explain. Reply OK.", "", 27},
		{"combining", "Public sample: e\u0301 👨‍👩‍👧‍👦. Reply OK.", "", 38},
		{"empty", "", "Reply OK.", 25},
		{"repeat", strings.Repeat("public sample ", 512) + "Reply OK.", "", 1045},
	}
	for _, model := range Models {
		for _, c := range cases {
			t.Run(model.Key+"/"+c.name, func(t *testing.T) {
				rq := &anthropic.Request{Model: model.ID, Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: c.text}}}}}
				if c.system != "" {
					rq.System, _ = json.Marshal(c.system)
				}
				built, err := BuildRequest(rq)
				if err != nil {
					t.Fatal(err)
				}
				got, err := CountInput(built)
				if err != nil || got != c.want {
					t.Fatalf("local=%d observed backend=%d err=%v", got, c.want, err)
				}
			})
		}
	}
}

func TestLocalTextCounterRejectsUnvalidatedInputs(t *testing.T) {
	cases := map[string]*Request{
		"tools":      {Tools: []ToolSpec{{Name: "Read"}}},
		"media":      {Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_image", ImageURL: "data:image/png;base64,AA=="}}}}},
		"reasoning":  {Input: []InputEntry{{Reasoning: &Reasoning{Encrypted: "synthetic"}}}},
		"structured": {Text: &TextParam{}},
		"unbroken":   {Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: strings.Repeat("a", 513)}}}}},
		"special":    {Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: "<|endoftext|>"}}}}},
	}
	for name, rq := range cases {
		t.Run(name, func(t *testing.T) {
			rq.Model = Models[0].ID
			rq.Instruction = Instruction
			if _, err := CountInput(rq); !errors.Is(err, ErrTokenCountUnsupported) {
				t.Fatalf("unvalidated input counted: %v", err)
			}
		})
	}
}

func BenchmarkLocalTextCount(b *testing.B) {
	rq := &Request{Model: Models[0].ID, Instruction: Instruction, Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: strings.Repeat("public sample ", 512)}}}}}
	if _, err := CountInput(rq); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := CountInput(rq); err != nil {
			b.Fatal(err)
		}
	}
}
