package devcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The file probe answers one question nothing offline can: does this backend read an
// attached file at all?
//
// It matters because the client can send one. claude 2.1.274 carries "document",
// application/pdf and pdf_page, so a Read of a PDF arrives here as a document block and
// this build refuses it as UNSUPPORTED_CONTENT -- the turn dies. Implementing it means
// choosing a wire shape, and the reference Codex client is no help: its binary has
// input_image but neither input_file nor file_data, so nothing local says what this
// backend accepts. Asking costs one attempt, and a "no" is worth that.
//
// The shape under test is the public Responses API's: an input_file part carrying a data
// URL. A refusal does not mean PDFs are impossible, it means not this shape, and the
// category says which.

// probePDF is a 592-byte one-page PDF generated for this probe by writing the objects and
// the xref table directly. It carries one generated token and nothing else, and describes
// nothing about this machine.
const probePDF = "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUl0gL0NvdW50IDEgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA2MTIgNzkyXSAvUmVzb3VyY2VzIDw8IC9Gb250IDw8IC9GMSA0IDAgUiA+PiA+PiAvQ29udGVudHMgNSAwIFIgPj4KZW5kb2JqCjQgMCBvYmoKPDwgL1R5cGUgL0ZvbnQgL1N1YnR5cGUgL1R5cGUxIC9CYXNlRm9udCAvSGVsdmV0aWNhID4+CmVuZG9iago1IDAgb2JqCjw8IC9MZW5ndGggNDggPj4Kc3RyZWFtCkJUIC9GMSAyNCBUZiA3MiA3MDAgVGQgKENMQVVEVUNULVBERi03UTRNKSBUaiBFVAplbmRzdHJlYW0KZW5kb2JqCnhyZWYKMCA2CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwOSAwMDAwMCBuIAowMDAwMDAwMDU4IDAwMDAwIG4gCjAwMDAwMDAxMTUgMDAwMDAgbiAKMDAwMDAwMDI0MSAwMDAwMCBuIAowMDAwMDAwMzExIDAwMDAwIG4gCnRyYWlsZXIKPDwgL1NpemUgNiAvUm9vdCAxIDAgUiA+PgpzdGFydHhyZWYKNDA5CiUlRU9GCg=="

// probePDFToken is what the PDF says. The model has no other way to know it, so a reply
// carrying it separates "the request was accepted" from "the file was read".
const probePDFToken = "CLAUDUCT-PDF-7Q4M"

func fileProbe(transport upstream.Transport, budget upstream.Budget, out io.Writer) int {
	const prompt = "The attached PDF contains one token. Reply with that token and nothing else."

	// The document block goes through the product's own decoder and bridge, which is the
	// point: an earlier version of this probe attached the file part by hand and so proved
	// only that the backend can read a PDF, not that this build sends one.
	request, err := anthropic.DecodeRequest([]byte(probeHead(budget, 256) +
		`"messages":[{"role":"user","content":[{"type":"text","text":` + quote(prompt) + `},` +
		`{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":` +
		quote(probePDF) + `}}]}]}`))
	if err != nil {
		fmt.Fprintln(out, "file           REFUSED REQUEST_DECODE", err)
		return 1
	}
	backend, err := bridge.BuildRequest(request)
	if err != nil {
		fmt.Fprintln(out, "file           REFUSED BUILD_REQUEST", err)
		return 1
	}
	body, err := json.Marshal(backend)
	if err != nil {
		fmt.Fprintln(out, "file           REFUSED ENCODE", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	response, err := transport.Execute(ctx, upstream.Call{
		Body:      body,
		Requested: request.Model,
		Model:     backend.Model,
		Effort:    backend.Effort.Effort,
		Source:    backend.Source,
	})
	if err != nil {
		result := exchange{category: categoryOf(err)}
		var failure upstream.Failure
		if errors.As(err, &failure) {
			result.status = failure.Status
		}
		fmt.Fprintln(out, "file          ", result)
		fmt.Fprintln(out, "              ", len(body), "bytes sent as input_file with a data URL")
		return 1
	}
	defer response.Body.Close()

	// The token is looked for in the bytes on their way past, not in the exchange the
	// translator returns. That buffer is deliberately cleared before it comes back
	// (wire.go, at EOF), so a check against it can never find anything -- which is exactly
	// what the first run of this probe reported, and it reported it as a fact about the
	// backend. Scanning the stream cannot be defeated that way.
	sighting := &sighting{want: []byte(probePDFToken)}
	original := response.Body
	response.Body = teed{Reader: io.TeeReader(original, sighting), Closer: original}

	result := translate(response, request)
	fmt.Fprintln(out, "file          ", result)
	switch {
	case !result.ok:
		return 1
	case sighting.found:
		fmt.Fprintln(out, "               the model returned the token, so the file was read")
		return 0
	default:
		// Accepted and not read is its own answer, and the one worth naming: a backend
		// that takes the part and ignores it would let this build ship a feature that
		// quietly answers about nothing.
		fmt.Fprintln(out, "               accepted, but the reply does not carry the token "+
			"-- not evidence the file was read")
		// The reply is printed here and nowhere else in this command. "Accepted and
		// ignored" and "accepted and refused out loud" lead to different decisions, and
		// the only thing that tells them apart is what the model said. Bounded, and the
		// content is a model's answer about a file this probe generated.
		fmt.Fprintf(out, "               said: %q", printable(sighting.head(), 200))
		fmt.Fprintln(out)
		return 1
	}
}

// teed is a ReadCloser whose reads are observed and whose Close still reaches the socket.
type teed struct {
	io.Reader
	io.Closer
}

// sighting reports whether a token went past, without keeping the stream.
//
// A bounded tail is kept because a token can straddle two reads, and a short prefix is
// kept so a probe that did not find it can still say what arrived instead.
type sighting struct {
	want   []byte
	found  bool
	tail   []byte
	prefix []byte
}

func (s *sighting) Write(p []byte) (int, error) {
	if !s.found {
		if bytes.Contains(append(append([]byte{}, s.tail...), p...), s.want) {
			s.found = true
		}
		keep := len(s.want)
		joined := append(append([]byte{}, s.tail...), p...)
		if len(joined) > keep {
			joined = joined[len(joined)-keep:]
		}
		s.tail = joined
	}
	if len(s.prefix) < 4096 {
		s.prefix = append(s.prefix, p...)
	}
	return len(p), nil
}

func (s *sighting) head() []byte { return s.prefix }

// printable bounds what is shown and drops control characters, so a backend cannot move
// the cursor or clear the screen of whoever ran the probe.
func printable(reply []byte, limit int) string {
	if len(reply) > limit {
		reply = reply[:limit]
	}
	out := make([]rune, 0, limit)
	for _, r := range string(reply) {
		if r < 32 || r == 127 {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}
