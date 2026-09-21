package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeReadPDFPagesUsesBundledWindowsRenderer(t *testing.T) {
	buildHook(t)
	_, cwd := workspace(t)
	data, err := os.ReadFile("testdata/public-vector.pdf")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cwd, "public.pdf")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"file_path": file, "pages": "1"})
	script := newScript(toolStream("read_public_pdf", "Read", string(args)), textStream("done", "PDF received"))
	out := (nativeRun{Cwd: cwd, Args: []string{"-p", "Read the public PDF page", "--allowedTools", "Read"}, transport: script}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || out.result.Diagnostics.NativeToolFailures.Total != 0 {
		t.Fatalf("native PDF Read failed: %v %s", out.err, tail(out.output(), 1600))
	}
	found := false
	for _, body := range script.Conversations() {
		var req struct {
			Input []struct {
				Output []struct {
					Type string `json:"type"`
					URL  string `json:"image_url"`
				}
			}
		}
		if json.Unmarshal([]byte(body), &req) != nil {
			t.Fatal("request decode")
		}
		for _, entry := range req.Input {
			for _, part := range entry.Output {
				if part.Type != "input_image" {
					continue
				}
				raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(part.URL, "data:image/jpeg;base64,"))
				if err != nil {
					t.Fatal(err)
				}
				im, err := jpeg.Decode(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				r, g, b, _ := im.At(im.Bounds().Dx()/2, im.Bounds().Dy()/2).RGBA()
				if r < 55000 || g > 5000 || b > 5000 {
					t.Fatal("native PDF page lost its red pixels")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("native Read emitted no page image")
	}
}
