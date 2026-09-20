//go:build policy_evidence && windows

package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
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

type countSample struct {
	name     string
	messages []anthropic.Message
	tools    []anthropic.Tool
}

func publicImage(format string, w, h int) string {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetRGBA(x, y, color.RGBA{R: 240, A: 255})
		}
	}
	var b bytes.Buffer
	switch format {
	case "png":
		_ = png.Encode(&b, im)
	case "jpeg":
		_ = jpeg.Encode(&b, im, nil)
	case "gif":
		_ = gif.Encode(&b, im, nil)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
func publicPDF(pages int, visual ...bool) string {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", ""}
	var kids []string
	for i := 0; i < pages; i++ {
		page := len(objects) + 1
		content := page + 1
		font := page + 2
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		text := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (PUBLIC-PDF-%d) Tj ET", i+1)
		if len(visual) > 0 && visual[0] {
			text = "1 0 0 rg 72 200 400 400 re f"
		}

		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", font, content), fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(text), text), "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
func TestPolicyEvidenceMultimodalCountMatrix(t *testing.T) {
	if os.Getenv("CLAUDUCT_MULTIMODAL_EVIDENCE") != "1" {
		t.Skip("explicit bounded live switch absent")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	user := func(text string) []anthropic.Message {
		return []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: text}}}}
	}
	// Parse through the real Anthropic decoder; no media or framing bypass.
	media := func(kind, mime, data string, tool bool) []anthropic.Message {
		block := map[string]any{"type": kind, "source": map[string]string{"type": "base64", "media_type": mime, "data": data}}
		var messages []any
		if tool {
			messages = []any{map[string]any{"role": "user", "content": "Inspect the public attachment and reply OK."}, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "call_public", "name": "Read", "input": map[string]string{"file_path": "public-fixture"}}}}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call_public", "content": []any{map[string]string{"type": "text", "text": "Public fixture. Reply OK."}, block}}}}}
		} else {
			messages = []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "text", "text": "Inspect the public attachment and reply OK."}, block}}}
		}
		raw, _ := json.Marshal(map[string]any{"model": "gpt-6-astra", "max_tokens": 64, "stream": true, "messages": messages})
		decoded, e := anthropic.DecodeRequest(raw)
		if e != nil {
			t.Fatal("fixture decode failed")
		}
		return decoded.Messages
	}
	cases := []countSample{
		{"english", user("Public sample. Reply OK."), nil},
		{"multilingual", user("공개 시험입니다. 日本語 中文 العربية हिन्दी Ελληνικά. Reply OK."), nil},
		{"unicode", user("Public: 👩🏽‍💻 e\u0301 é 한글\t\n\r\n a  b \u00a0 z. Reply OK."), nil},
		{"long-piece", user(strings.Repeat("abcdefgh", 128) + " Reply OK."), nil},
		{"special-spelling", user("Public literal <|endoftext|> and <|im_start|>. Reply OK."), nil},
		{"turns", []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Public first."}}}, {Role: "assistant", Blocks: []anthropic.Block{{Type: "text", Text: "Acknowledged."}}}, {Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply OK."}}}}, nil},
		{"png-small", media("image", "image/png", publicImage("png", 64, 64), false), nil},
		{"png-large", media("image", "image/png", publicImage("png", 2048, 1536), false), nil},
		{"jpeg", media("image", "image/jpeg", publicImage("jpeg", 1024, 768), false), nil},
		{"gif", media("image", "image/gif", publicImage("gif", 96, 96), false), nil},
		{"pdf-one", media("document", "application/pdf", publicPDF(1), false), nil},
		{"pdf-two", media("document", "application/pdf", publicPDF(2), false), nil},
		{"image-tool-result", media("image", "image/png", publicImage("png", 64, 64), true), []anthropic.Tool{{Name: "Read", InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`)}}},
		{"pdf-tool-result", media("document", "application/pdf", publicPDF(1), true), []anthropic.Tool{{Name: "Read", InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`)}}},
	}
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			t.Parallel()
			ledger := NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 2 * len(cases)})
			direct := NewDirect(&auth.Provider{}, ledger, Fixed(version))
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			for _, sample := range cases {
				request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: sample.messages, Tools: sample.tools})
				if err != nil {
					t.Fatal("request conversion failed")
				}
				if len(sample.tools) > 0 {
					request.ToolChoice = "none"
				}
				raw, _ := json.Marshal(request)
				start := time.Now()
				count, err := direct.Count(ctx, Call{Model: model.ID, Effort: "low", Body: raw})
				if err != nil {
					t.Fatalf("case=%s backend_count_failed category=%v; no retry", sample.name, err)
				}
				ms := time.Since(start).Milliseconds()
				local, localErr := bridge.CountInput(request)
				response, err := direct.Execute(ctx, Call{Model: model.ID, Effort: "low", Body: raw, Source: "public-multimodal-count"})
				if err != nil {
					t.Fatalf("case=%s generation_failed; no retry", sample.name)
				}
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				response.Body.Close()
				if err != nil || len(data) >= 1<<20 {
					t.Fatalf("case=%s stream_failed; no retry", sample.name)
				}
				var actual *int64
				for _, line := range bytes.Split(data, []byte("\n")) {
					if !bytes.HasPrefix(line, []byte("data: ")) {
						continue
					}
					var e struct {
						Type     string
						Response struct {
							Usage struct {
								Input *int64 `json:"input_tokens"`
							}
						}
					}
					if json.Unmarshal(line[6:], &e) == nil && e.Type == "response.completed" {
						actual = e.Response.Usage.Input
					}
				}
				if actual == nil {
					t.Fatalf("case=%s completed_usage_absent; no retry", sample.name)
				}
				t.Logf("case=%s counted=%d actual=%d count_ms=%d local_supported=%t local=%d", sample.name, count, *actual, ms, localErr == nil, local)
				if count != *actual {
					t.Fatalf("case=%s BACKEND_COUNT_MISMATCH", sample.name)
				}
				if localErr == nil && local != *actual {
					t.Fatalf("case=%s LOCAL_COUNT_MISMATCH", sample.name)
				}
			}
			attempts, _, _ := ledger.Spent()
			if attempts != len(cases)*2 {
				t.Fatal("unexpected retry/attempt count")
			}
		})
	}
}

// The reply must come from visual content, not from text in the user prompt.
// Exactly eight requests per model; no tools are executed and no retries pass.
func TestPolicyEvidenceMediaPerception(t *testing.T) {
	webpOnly := os.Getenv("CLAUDUCT_WEBP_PERCEPTION") == "1"
	if os.Getenv("CLAUDUCT_MEDIA_PERCEPTION") != "1" && !webpOnly {
		t.Skip("explicit bounded live switch absent")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			t.Parallel()
			limit := 8
			if webpOnly {
				limit = 2
			}
			ledger := NewLedger(Budget{Model: model.ID, Effort: "low", Limit: limit})
			direct := NewDirect(&auth.Provider{}, ledger, Fixed(version))
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			samples := []struct{ name, kind, mime, data string }{
				{"png-color", "input_image", "image/png", publicImage("png", 128, 128)},
				{"jpeg-color", "input_image", "image/jpeg", publicImage("jpeg", 1024, 768)},
				{"pdf-vector-color", "input_file", "application/pdf", publicPDF(1, true)},
				{"pdf-text", "input_file", "application/pdf", publicPDF(1)},
			}
			if webpOnly {
				data, err := os.ReadFile("testdata/public-blossoms.webp")
				if err != nil {
					t.Fatal(err)
				}
				samples = []struct{ name, kind, mime, data string }{{"webp-blossoms", "input_image", "image/webp", base64.StdEncoding.EncodeToString(data)}}
			}
			for _, sample := range samples {
				prompt := "What is the solid rectangle's fill color? Answer one lowercase English word only."
				want := "red"
				if sample.name == "pdf-text" {
					prompt = "Read the label in the document. Reply with that exact label only."
					want = "PUBLIC-PDF-1"
				}
				if webpOnly {
					prompt = "Does this image contain flowers? Reply with exactly yes or no in lowercase."
					want = "yes"
				}
				request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: prompt}}}}})
				if err != nil {
					t.Fatal("build failed")
				}
				part := bridge.InputPart{Type: sample.kind}
				if sample.kind == "input_image" {
					part.ImageURL = "data:" + sample.mime + ";base64," + sample.data
				} else {
					part.Filename = "public.pdf"
					part.FileData = "data:" + sample.mime + ";base64," + sample.data
				}
				parts := []bridge.InputPart{part}
				if sample.kind == "input_file" {
					data, err := base64.StdEncoding.DecodeString(sample.data)
					if err != nil {
						t.Fatal(err)
					}
					pages, err := pdf.Render(ctx, data)
					if err != nil {
						t.Fatal(err)
					}
					for index, page := range pages {
						parts = append(parts, bridge.InputPart{Type: "input_text", Text: fmt.Sprintf("PDF page %d (rendered visual content):", index+1)}, bridge.InputPart{Type: "input_image", ImageURL: page})
					}
				}
				request.Input = append(request.Input, bridge.InputEntry{Role: "user", Content: parts})
				raw, _ := json.Marshal(request)
				count, err := direct.Count(ctx, Call{Model: model.ID, Effort: "low", Body: raw})
				if err != nil {
					t.Fatalf("case=%s count_failed", sample.name)
				}
				response, err := direct.Execute(ctx, Call{Model: model.ID, Effort: "low", Body: raw, Source: "public-media-perception"})
				if err != nil {
					t.Fatalf("case=%s generation_failed", sample.name)
				}
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				response.Body.Close()
				if err != nil {
					t.Fatal("stream read failed")
				}
				var text strings.Builder
				var actual *int64
				for _, line := range bytes.Split(data, []byte("\n")) {
					if !bytes.HasPrefix(line, []byte("data: ")) {
						continue
					}
					var e struct {
						Type, Delta string
						Response    struct {
							Usage struct {
								Input *int64 `json:"input_tokens"`
							}
						}
					}
					if json.Unmarshal(line[6:], &e) != nil {
						continue
					}
					if e.Type == "response.output_text.delta" {
						text.WriteString(e.Delta)
					}
					if e.Type == "response.completed" {
						actual = e.Response.Usage.Input
					}
				}
				visible := strings.Trim(strings.TrimSpace(text.String()), " .\n\r\t\x60\"")
				t.Logf("case=%s count=%d count_agreement=%t expected_content=%t", sample.name, count, actual != nil && *actual == count, visible == want)
				if actual == nil || *actual != count {
					t.Errorf("case=%s count_mismatch", sample.name)
				}
				if visible != want {
					t.Errorf("case=%s expected_content_missing; no retry", sample.name)
				}
			}
			attempts, _, _ := ledger.Spent()
			if attempts != limit {
				t.Fatal("unexpected retry/attempt count")
			}
		})
	}
}
