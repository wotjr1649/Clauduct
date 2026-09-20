package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNativePDFRefusesUnknownOptionsAndOutsideTempOutput(t *testing.T) {
	var out, log bytes.Buffer
	if nativePDF([]string{"-v"}, &out, &log) != 0 || out.Len() == 0 {
		t.Fatal("version probe failed")
	}
	for _, args := range [][]string{{"-shell", "anything"}, {"-jpeg", "-r", "100", "-r", "100", "C:/missing.pdf", "C:/out"}, {"-jpeg", "-r", "100", "C:/missing.pdf", filepath.Join(t.TempDir(), "..", "..", "outside-prefix")}, {"-jpeg", "-r", "999", "C:/missing.pdf", "C:/out"}} {
		if nativePDF(args, &out, &log) == 0 {
			t.Fatal("unsafe request accepted")
		}
	}
}

func TestNativePDFSupportsOnlyTheConfiguredInteractiveSidecarScope(t *testing.T) {
	t.Setenv("TEMP", t.TempDir())
	t.Setenv("TMP", os.Getenv("TEMP"))
	projects := t.TempDir()
	t.Setenv("CLAUDUCT_PDF_PROJECTS_ROOT", projects)
	dir := filepath.Join(projects, "public-project", "public-session", "tool-results", "pdf-public")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	input, err := filepath.Abs("../../internal/app/testdata/public-vector.pdf")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-jpeg", "-r", "100", "-f", "1", "-l", "1", input, filepath.Join(dir, "page")}
	var out, log bytes.Buffer
	if code := nativePDF(args, &out, &log); code != 0 {
		t.Fatalf("valid sidecar failed: %d %s", code, log.String())
	}
	if info, err := os.Stat(filepath.Join(dir, "page-000001.jpg")); err != nil || info.Size() == 0 {
		t.Fatal("rendered page missing")
	}
	if nativePDF(args, &out, &log) == 0 {
		t.Fatal("existing sidecar overwritten")
	}
	args[len(args)-1] = filepath.Join(projects, "outside-native-sidecar")
	if nativePDF(args, &out, &log) == 0 {
		t.Fatal("arbitrary profile write allowed")
	}
}
