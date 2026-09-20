package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The path a PDF actually takes: the user asks, the client runs Read, and the file comes
// back as a document block inside a tool result.
//
// Every other check of this was one layer short. The bridge test builds the block by hand;
// the probe attaches the part itself and asks the backend. Neither shows that the real
// client produces a document block for a real PDF, or that this build carries the one it
// produces. That is what this measures, and it costs no model call.
func TestReadingAPDFCarriesItToTheBackend(t *testing.T) {
	buildHook(t) // PDF rendering is provided by the shipped companion executable.
	pdf, err := base64.StdEncoding.DecodeString(onePagePDFBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	script, _, _ := scripted(t,
		[]string{"-p", "read the pdf and tell me what it says", "--allowedTools", "Read"},
		toolStream("call_read_1", "Read", `{"file_path":"`+filepath.ToSlash(path)+`"}`),
		textStream("resp_done", "done"))

	conversations := script.Conversations()
	if len(conversations) < 2 {
		t.Fatalf("conversation requests = %d, want the call and its result", len(conversations))
	}
	// The second one carries the tool result, which is where the document rides.
	withResult := conversations[len(conversations)-1]
	for _, want := range []string{
		`"type":"input_file"`,
		`"filename":"document.pdf"`,
		`"file_data":"data:application/pdf;base64,JVBER`,
		`"type":"input_image"`,
		`"image_url":"data:image/png;base64,iVBOR`,
	} {
		if !strings.Contains(withResult, want) {
			// Either the client did not return the PDF as a document block, or this build
			// dropped it. The tail shows which: the conversation is at the end.
			t.Fatalf("the request carrying the tool result has no %s; tail: %s",
				want, tail(withResult, 400))
		}
	}
}

// onePagePDFBase64 is the same generated one-page PDF the probe sends: objects and xref
// written directly, one token inside, nothing about this machine.
const onePagePDFBase64 = "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUl0gL0NvdW50IDEgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA2MTIgNzkyXSAvUmVzb3VyY2VzIDw8IC9Gb250IDw8IC9GMSA0IDAgUiA+PiA+PiAvQ29udGVudHMgNSAwIFIgPj4KZW5kb2JqCjQgMCBvYmoKPDwgL1R5cGUgL0ZvbnQgL1N1YnR5cGUgL1R5cGUxIC9CYXNlRm9udCAvSGVsdmV0aWNhID4+CmVuZG9iago1IDAgb2JqCjw8IC9MZW5ndGggNDggPj4Kc3RyZWFtCkJUIC9GMSAyNCBUZiA3MiA3MDAgVGQgKENMQVVEVUNULVBERi03UTRNKSBUaiBFVAplbmRzdHJlYW0KZW5kb2JqCnhyZWYKMCA2CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwOSAwMDAwMCBuIAowMDAwMDAwMDU4IDAwMDAwIG4gCjAwMDAwMDAxMTUgMDAwMDAgbiAKMDAwMDAwMDI0MSAwMDAwMCBuIAowMDAwMDAwMzExIDAwMDAwIG4gCnRyYWlsZXIKPDwgL1NpemUgNiAvUm9vdCAxIDAgUiA+PgpzdGFydHhyZWYKNDA5CiUlRU9GCg=="
