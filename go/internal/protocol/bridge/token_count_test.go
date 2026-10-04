package bridge

import (
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"strings"
	"testing"
)

// An account model this build never measured routes and is counted by the backend, but
// never inherits the local formula validated for the legacy models.
func TestNewRouteDoesNotInheritCountValidation(t *testing.T) {
	catalogue, _, err := NewCatalogue([]AccountModel{{ID: "future-public-fixture", Efforts: []string{"low"}, Default: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	request := &Request{Model: "future-public-fixture", Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: "Reply OK."}}}}}
	if _, err := (Selection{}).WithCatalogue(catalogue).SelectRoute(request.Model, "low"); err != nil {
		t.Fatal("fixture route unavailable")
	}
	if _, err := CountInput(request); !errors.Is(err, ErrTokenCountUnsupported) {
		t.Fatal("routing enabled unvalidated local token counting")
	}
	if !BackendCountSupported(request) {
		t.Fatal("the backend count refused a model only because this build never measured it")
	}
}

func TestLocalTextCounterMatchesRecordedBackendCounts(t *testing.T) {
	// Expected values are backend response.completed usage, not tokenizer output: recorded
	// with the fixed instructions, less the 12 tokens their removal took off every one of 16
	// cells (#144), and re-read without them on luna (maintainer's evidence, 2026-09-26).
	cases := []struct {
		name, text, system string
		want               int64
	}{
		{"short", "Reply OK.", "", 9},
		{"english", "Public synthetic token test: the quick brown fox jumps over the lazy dog. Reply OK.", "", 24},
		{"korean", "공개 합성 테스트입니다. 모델별 입력 토큰 수를 확인합니다. OK만 답하세요.", "", 28},
		{"unicode", "Public sample: café naïve 東京 😀. Reply OK.", "", 18},
		{"code", "Public code sample: func add(a, b int) int { return a + b }\nReply OK.", "", 27},
		{"system", "Reply OK.", "This is a public synthetic tokenizer test. Answer briefly.", 24},
		{"space", "  Reply OK.  ", "", 11},
		{"lines", "\n\nReply\tOK.\r\n", "", 11},
		{"contractions", "We're testing; don't explain. Reply OK.", "", 15},
		{"combining", "Public sample: e\u0301 👨‍👩‍👧‍👦. Reply OK.", "", 26},
		{"empty", "", "Reply OK.", 13},
		{"repeat", strings.Repeat("public sample ", 512) + "Reply OK.", "", 1033},
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
			if _, err := CountInput(rq); !errors.Is(err, ErrTokenCountUnsupported) {
				t.Fatalf("unvalidated input counted: %v", err)
			}
		})
	}
}

func BenchmarkLocalTextCount(b *testing.B) {
	rq := &Request{Model: Models[0].ID, Input: []InputEntry{{Role: "user", Content: []InputPart{{Type: "input_text", Text: strings.Repeat("public sample ", 512)}}}}}
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
