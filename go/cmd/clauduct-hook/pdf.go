package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Native 2.1.276 invokes exactly -v, or -jpeg -r 100 [-f N] [-l N]
// file output-prefix. This is a bounded compatibility subset, not Poppler.
// Native Read still owns file permission checks. Output is exclusive and must
// remain inside its temporary directory; no shell or arbitrary options execute.
func nativePDF(args []string, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "-v" {
		fmt.Fprintln(out, "Clauduct Windows PDF renderer 1")
		return 0
	}
	first, last, dpi := 1, 0, 100
	if len(args) < 5 || args[0] != "-jpeg" {
		fmt.Fprintln(errOut, "Command Line Error: unsupported renderer options")
		return 2
	}
	values := args[1:]
	seen := map[string]bool{}
	for len(values) > 2 {
		flag := values[0]
		if seen[flag] || (flag != "-f" && flag != "-l" && flag != "-r") {
			fmt.Fprintln(errOut, "Command Line Error: unsupported renderer options")
			return 2
		}
		seen[flag] = true
		n, err := strconv.Atoi(values[1])
		if err != nil || n < 1 {
			fmt.Fprintln(errOut, "Command Line Error: invalid page or density")
			return 2
		}
		switch flag {
		case "-f":
			first = n
		case "-l":
			last = n
		case "-r":
			dpi = n
		}
		values = values[2:]
	}
	if len(values) != 2 || !filepath.IsAbs(values[0]) || !filepath.IsAbs(values[1]) || dpi < 36 || dpi > 300 {
		fmt.Fprintln(errOut, "Command Line Error: invalid renderer request")
		return 2
	}
	outputRoot := os.TempDir()
	relative, err := filepath.Rel(outputRoot, values[1])
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		// Interactive native 2.1.276 persists page sidecars beneath its configured
		// projects root; print mode uses TEMP. Require the exact sidecar shape.
		outputRoot = os.Getenv("CLAUDUCT_PDF_PROJECTS_ROOT")
		if filepath.IsAbs(outputRoot) {
			relative, err = filepath.Rel(outputRoot, values[1])
		} else {
			err = fmt.Errorf("missing output root")
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if err == nil && (len(parts) != 5 || !identifier.MatchString(parts[1]) || parts[2] != "tool-results" || !strings.HasPrefix(parts[3], "pdf-") || !identifier.MatchString(parts[3]) || parts[4] != "page") {
			err = fmt.Errorf("invalid output scope")
		}
	}
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		fmt.Fprintln(errOut, "Permission Error: output is outside native temporary directory")
		return 2
	}
	root, err := os.OpenRoot(outputRoot)
	if err != nil {
		fmt.Fprintln(errOut, "PDF_OUTPUT_UNAVAILABLE")
		return 1
	}
	defer root.Close()
	input, err := os.Open(values[0])
	if err != nil {
		fmt.Fprintln(errOut, "PDF_INPUT_UNAVAILABLE")
		return 1
	}
	defer input.Close()
	stat, err := input.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > pdf.MaxBytes {
		fmt.Fprintln(errOut, "PDF_INPUT_LIMIT")
		return 1
	}
	data, err := io.ReadAll(io.LimitReader(input, pdf.MaxBytes+1))
	if err != nil || len(data) > pdf.MaxBytes {
		fmt.Fprintln(errOut, "PDF_INPUT_LIMIT")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pages, err := pdf.RenderRange(ctx, data, first, last, dpi)
	if err != nil {
		// Only the native engine's fixed categories and numeric range message travel.
		if strings.HasPrefix(err.Error(), "Wrong page range given:") {
			fmt.Fprintln(errOut, err.Error())
		} else {
			fmt.Fprintln(errOut, "PDF_RENDER_FAILED")
		}
		return 1
	}
	for index, page := range pages {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(page, "data:image/png;base64,"))
		if err != nil {
			return 1
		}
		image, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			return 1
		}
		name := relative + fmt.Sprintf("-%06d.jpg", first+index)
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintln(errOut, "PDF_OUTPUT_REFUSED")
			return 1
		}
		writeErr := jpeg.Encode(file, image, &jpeg.Options{Quality: 90})
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			fmt.Fprintln(errOut, "PDF_OUTPUT_FAILED")
			return 1
		}
	}
	return 0
}
