//go:build policy_evidence && windows

package upstream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Separate public inputs discriminate raw-file processing from image processing.
// One count and one generation per case. No retries to erase a mismatch.
func TestPublicMixedPDFCount(t *testing.T) {
	if os.Getenv("CLAUDUCT_MIXED_PDF_EVIDENCE") != "1" {
		t.Skip("explicit live switch absent")
	}
	data, err := os.ReadFile("../../../verification/release-investigation-20260920/mixed-report-s45.pdf")
	if err != nil || len(data) > 16000 {
		t.Fatal("public fixture unavailable")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != "d76e570eaabac43021acde46fe05bcb2dec00492e13e0eec6962ad5a5ed32fb4" {
		t.Fatal("public fixture digest mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	pages, err := pdf.Render(ctx, data)
	if err != nil {
		t.Fatal("render failed")
	}
	low, err := pdf.RenderRange(ctx, data, 1, 3, 100)
	if err != nil {
		t.Fatal("native density render failed")
	}
	var highParts, lowParts, nativeParts []bridge.InputPart
	for _, p := range pages {
		highParts = append(highParts, bridge.InputPart{Type: "input_image", ImageURL: p})
	}
	for _, p := range low {
		b, e := base64.StdEncoding.DecodeString(p[len("data:image/png;base64,"):])
		if e != nil {
			t.Fatal("png decode")
		}
		im, e := png.Decode(bytes.NewReader(b))
		if e != nil {
			t.Fatal("png image")
		}
		var out bytes.Buffer
		if jpeg.Encode(&out, im, nil) != nil {
			t.Fatal("jpeg encode")
		}
		lowParts = append(lowParts, bridge.InputPart{Type: "input_image", ImageURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes())})
		out.Reset()
		if jpeg.Encode(&out, im, &jpeg.Options{Quality: 90}) != nil {
			t.Fatal("native jpeg encode")
		}
		nativeParts = append(nativeParts, bridge.InputPart{Type: "input_image", ImageURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes())})
	}
	file := bridge.InputPart{Type: "input_file", Filename: "public.pdf", FileData: "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(data)}
	cases := []struct {
		name  string
		parts []bridge.InputPart
	}{
		{"file_only", []bridge.InputPart{file}},
		{"png_192_three", highParts},
		{"jpeg_100_three", lowParts},
		{"file_and_png", append([]bridge.InputPart{file}, highParts...)},
		{"jpeg90_user", nativeParts},
		{"jpeg90_tool", nativeParts},
		{"jpeg90_tool_high", nativeParts},
		{"jpeg90_tool_auto", nativeParts},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ledger := NewLedger(Budget{Model: "gpt-6-astra", Effort: "low", Limit: 2})
			d := NewDirect(&auth.Provider{}, ledger, Fixed(version))
			defer d.Close()
			r, e := bridge.BuildRequest(&anthropic.Request{Model: "gpt-6-astra", Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Inspect the public report. Reply with only OK."}}}}})
			if e != nil {
				t.Fatal("build")
			}
			if strings.HasPrefix(c.name, "jpeg90_tool") {
				r.Input = append(r.Input, bridge.InputEntry{Type: "function_call", CallID: "call_public_pdf", Name: "Read", Arguments: `{"file_path":"mixed-report-s45.pdf"}`}, bridge.InputEntry{Type: "function_call_output", CallID: "call_public_pdf", Output: c.parts})
			} else {
				r.Input = append(r.Input, bridge.InputEntry{Role: "user", Content: c.parts})
			}
			raw, e := json.Marshal(r)
			if e != nil {
				t.Fatal("encode")
			}
			if strings.HasSuffix(c.name, "_high") || strings.HasSuffix(c.name, "_auto") {
				var obj map[string]any
				if json.Unmarshal(raw, &obj) != nil {
					t.Fatal("detail decode")
				}
				items := obj["input"].([]any)
				output := items[len(items)-1].(map[string]any)["output"].([]any)
				for _, p := range output {
					p.(map[string]any)["detail"] = strings.TrimPrefix(c.name, "jpeg90_tool_")
				}
				raw, e = json.Marshal(obj)
				if e != nil {
					t.Fatal("detail encode")
				}
			}
			call := Call{Model: r.Model, Effort: "low", Body: raw, Source: "public-mixed-pdf"}
			start := time.Now()
			count, e := d.Count(ctx, call)
			if e != nil {
				t.Fatal("count failed")
			}
			ms := time.Since(start).Milliseconds()
			resp, e := d.Execute(ctx, call)
			if e != nil {
				t.Fatal("generation failed")
			}
			body, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if e != nil || len(body) >= 1<<20 {
				t.Fatal("stream failed")
			}
			actual := int64(-1)
			for _, line := range bytes.Split(body, []byte("\n")) {
				if !bytes.HasPrefix(line, []byte("data: ")) {
					continue
				}
				var v struct {
					Type     string
					Response struct {
						Usage struct {
							Input int64 `json:"input_tokens"`
						}
					}
				}
				if json.Unmarshal(line[6:], &v) == nil && v.Type == "response.completed" {
					actual = v.Response.Usage.Input
				}
			}
			t.Logf("case=%s counted=%d actual=%d count_ms=%d payload_bytes=%d", c.name, count, actual, ms, len(raw))
			if actual != count {
				t.Error("COUNT_INPUT_MISMATCH")
			}
			a, _, _ := ledger.Spent()
			if a != 2 {
				t.Error("unexpected attempts")
			}
		})
	}
}
