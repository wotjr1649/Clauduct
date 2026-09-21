//go:build windows && amd64

package pdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
	"testing"
	"time"
)

func publicPDF() []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>"}
	content := "1 0 0 rg 0 0 100 100 re f"
	objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	var b strings.Builder
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
	return []byte(b.String())
}
func TestWindowsPDFRendersPixelsAndRejectsInvalidInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pages, err := Render(ctx, publicPDF())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatal("page count")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pages[0], "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := image.At(image.Bounds().Dx()/2, image.Bounds().Dy()/2).RGBA()
	if r < 60000 || g > 1000 || b > 1000 {
		t.Fatal("vector rectangle not rendered red")
	}
	if _, err := Render(ctx, []byte("%PDF-invalid")); err == nil {
		t.Fatal("invalid PDF accepted")
	}
}
