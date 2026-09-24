package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Existing input_file text is retained. The subscription backend was observed
// to omit PDF vectors/images, so render every page before both count and send.
// No subprocess or cache lock is involved in requests without a PDF.
type documentRenderer struct {
	mu     sync.Mutex
	helper string
	cache  map[[32]byte][]bridge.InputPart
	bytes  int
}

func (g *Gateway) ConfigurePDFRenderer(helper string) { g.documents.helper = helper }
func (g *Gateway) prepareDocuments(ctx context.Context, request *bridge.Request) error {
	for i := range request.Input {
		entry := &request.Input[i]
		if parts, ok := entry.Content.([]bridge.InputPart); ok {
			out, err := g.documentParts(ctx, parts)
			if err != nil {
				return err
			}
			entry.Content = out
		}
		if entry.Output != nil {
			out, err := g.documentParts(ctx, entry.Output)
			if err != nil {
				return err
			}
			entry.Output = out
		}
	}
	return nil
}
func (g *Gateway) documentParts(ctx context.Context, parts []bridge.InputPart) ([]bridge.InputPart, error) {
	var out []bridge.InputPart
	for index, part := range parts {
		if part.Type != "input_file" {
			if out != nil {
				out = append(out, part)
			}
			continue
		}
		if out == nil {
			out = append([]bridge.InputPart{}, parts[:index]...)
		}
		pages, err := g.documents.render(ctx, part.FileData)
		if err != nil {
			return nil, err
		}
		out = append(out, part)
		out = append(out, pages...)
	}
	if out == nil {
		return parts, nil
	}
	return out, nil
}

// ponytail: PDFs serialize behind one bounded cache; move to per-digest flights
// only if concurrent PDF profiling shows contention. Ordinary requests bypass it.
func (d *documentRenderer) render(ctx context.Context, dataURL string) ([]bridge.InputPart, error) {
	key := sha256.Sum256([]byte(dataURL))
	d.mu.Lock()
	defer d.mu.Unlock()
	if value, ok := d.cache[key]; ok {
		return value, nil
	}
	if !filepath.IsAbs(d.helper) {
		return nil, errors.New("PDF_RENDERER_UNAVAILABLE")
	}
	encoded, ok := strings.CutPrefix(dataURL, "data:application/pdf;base64,")
	if !ok || len(encoded) > base64.StdEncoding.EncodedLen(pdf.MaxBytes) {
		return nil, errors.New("PDF_INPUT_LIMIT")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("PDF_INPUT_INVALID")
	}
	bounded, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	cmd := exec.Command(d.helper, pdf.RenderArg)
	cmd.Stdin = bytes.NewReader(data)
	// A renderer receives no credentials, profile, PATH, prompt or user filenames.
	cmd.Env = []string{}
	for _, name := range []string{"SystemRoot", "WINDIR"} {
		if value := os.Getenv(name); value != "" {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Dir = filepath.Dir(d.helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdout, childOutput, err := os.Pipe()
	if err != nil {
		return nil, errors.New("PDF_RENDER_FAILED")
	}
	defer stdout.Close()
	cmd.Stdout = childOutput
	process, err := childprocess.Start(cmd)
	_ = childOutput.Close()
	if err != nil {
		return nil, errors.New("PDF_RENDER_FAILED")
	}
	stop := context.AfterFunc(bounded, func() { _ = process.Stop() })
	defer stop()
	raw, readErr := io.ReadAll(io.LimitReader(stdout, 32<<20+1))
	if readErr != nil || len(raw) > 32<<20 {
		cancel()
		_ = process.Stop()
		_ = process.Wait()
		return nil, errors.New("PDF_RENDER_SIZE_LIMIT")
	}
	if err = process.Wait(); err != nil {
		return nil, errors.New("PDF_RENDER_FAILED")
	}
	var urls []string
	if json.Unmarshal(raw, &urls) != nil || len(urls) == 0 || len(urls) > pdf.MaxPages {
		return nil, errors.New("PDF_RENDER_INVALID")
	}
	pages := make([]bridge.InputPart, 0, len(urls)*2)
	size := 0
	for index, url := range urls {
		if !strings.HasPrefix(url, "data:image/png;base64,iVBOR") {
			return nil, errors.New("PDF_RENDER_INVALID")
		}
		size += len(url)
		pages = append(pages, bridge.InputPart{Type: "input_text", Text: pageLabel(index)}, bridge.InputPart{Type: "input_image", ImageURL: url})
	}
	if d.cache == nil {
		d.cache = map[[32]byte][]bridge.InputPart{}
	}
	if len(d.cache) >= 16 || d.bytes+size > 32<<20 {
		clear(d.cache)
		d.bytes = 0
	}
	if size <= 32<<20 {
		d.cache[key] = pages
		d.bytes += size
	}
	return pages, nil
}
func pageLabel(index int) string {
	return "PDF page " + strconv.Itoa(index+1) + " (rendered visual content):"
}
