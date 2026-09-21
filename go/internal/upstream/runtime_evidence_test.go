//go:build runtime_evidence

package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Four models, four count-only requests each, no generation or retries. The
// entire outbound input is constructed here from public synthetic constants.
// A/B/A on one connection detects accidental retained conversation; fresh B
// separately checks the multimodal result. No credential/provider body is logged.
func TestRuntimeEvidenceReusedCountsEqualFreshBackendCounts(t *testing.T) {
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
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	for _, model := range bridge.Models {
		t.Run(model.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			makeCall := func(multimodal bool) Call {
				blocks := []anthropic.Block{{Type: "text", Text: "Public count probe: alpha. 공개 계수 검증."}}
				if multimodal {
					blocks = append(blocks, anthropic.Block{Type: "image", MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(encoded.Bytes())})
				}
				rq := &anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: blocks}}}
				built, e := bridge.BuildRequest(rq)
				if e != nil {
					t.Fatal("build rejected")
				}
				body, e := json.Marshal(built)
				if e != nil {
					t.Fatal("encode rejected")
				}
				return Call{Body: body, Requested: model.ID, Model: model.ID, Effort: "low", Source: "runtime-evidence"}
			}
			pooled := NewDirect(provider, NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 3}), Fixed(version))
			defer pooled.Close()
			fresh := NewDirect(provider, NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 1}), Fixed(version))
			defer fresh.Close()
			start := time.Now()
			a, e := pooled.Count(ctx, makeCall(false))
			if e != nil {
				t.Fatalf("initial count: %v", e)
			}
			b, e := pooled.Count(ctx, makeCall(true))
			if e != nil {
				t.Fatalf("reused multimodal count: %v", e)
			}
			again, e := pooled.Count(ctx, makeCall(false))
			if e != nil {
				t.Fatalf("reused text count: %v", e)
			}
			oracle, e := fresh.Count(ctx, makeCall(true))
			if e != nil {
				t.Fatalf("fresh multimodal count: %v", e)
			}
			stats := pooled.CountStats()
			t.Logf("text=%d text_again=%d image=%d image_fresh=%d dials=%d reused=%d elapsed_ms=%d", a, again, b, oracle, stats.Dials, stats.Reused, time.Since(start).Milliseconds())
			if a != again || b != oracle || stats.Dials != 1 || stats.Reused != 2 {
				t.Fatal("reused backend count was not verified")
			}
			_, inferences, _ := pooled.Ledger.Spent()
			if inferences != 0 {
				t.Fatal("count generated a response")
			}
		})
	}
}
