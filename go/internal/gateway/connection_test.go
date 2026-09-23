package gateway

import (
	"bufio"
	"context"

	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Exercise the product listener and response writer over real loopback sockets.
// No upstream request or credential is needed for the public model catalogue.
func TestGatewayResponsesSurviveConnectionTurnover(t *testing.T) {
	fixture := &upstream.Fixture{SSE: longStream(8)}
	g := startWith(t, fixture)
	for _, streaming := range []bool{false, true} {
		for _, closeEach := range []bool{false, true} {
			name := "reused"
			if closeEach {
				name = "closed"
			}
			if streaming {
				name += "/stream"
			} else {
				name += "/catalogue"
			}
			t.Run(name, func(t *testing.T) {
				transport := &http.Transport{Proxy: nil, DisableKeepAlives: closeEach}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
				failures := 0
				before := fixture.Calls()
				for i := 0; i < 200; i++ {
					req, err := http.NewRequest(http.MethodGet, g.BaseURL()+"/v1/models", nil)
					if streaming {
						req, err = http.NewRequest(http.MethodPost, g.BaseURL()+"/v1/messages", strings.NewReader(validRequest))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Anthropic-Version", anthropicVersion)
						req.Header.Set("X-Claude-Code-Request-Class", "main")
					}
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", "Bearer "+g.Token())
					started := time.Now()
					response, err := client.Do(req)
					if err != nil {
						failures++
						if failures <= 3 {
							t.Logf("iteration=%d elapsed=%s response_error=%v", i, time.Since(started), err)
						}
						continue
					}
					raw, readErr := io.ReadAll(response.Body)
					closeErr := response.Body.Close()
					valid := false
					var parseErr error
					text, stops := 0, 0
					if streaming {
						for _, line := range strings.Split(string(raw), "\n") {
							data, ok := strings.CutPrefix(line, "data: ")
							if !ok {
								continue
							}
							var frame struct {
								Type  string
								Delta struct{ Type, Text string }
							}
							if parseErr = json.Unmarshal([]byte(data), &frame); parseErr != nil {
								break
							}
							if frame.Type == "content_block_delta" && frame.Delta.Type == "text_delta" {
								text += len(frame.Delta.Text)
							}
							if frame.Type == "message_stop" {
								stops++
							}
						}
						valid = parseErr == nil && text == 8*8192 && stops == 1
					} else {
						var body struct{ Data []struct{ ID string } }
						parseErr = json.Unmarshal(raw, &body)
						valid = parseErr == nil && len(body.Data) > 0
					}
					if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK || !valid {
						failures++
						if failures <= 3 {
							t.Logf("iteration=%d elapsed=%s status=%d bytes=%d read_error=%v close_error=%v parse_error=%v text_bytes=%d stops=%d", i, time.Since(started), response.StatusCode, len(raw), readErr, closeErr, parseErr, text, stops)
						}
					}
				}
				t.Logf("requests=200 failures=%d", failures)
				if failures != 0 {
					t.Fatalf("product responses lost on %d connections", failures)
				}
				want := int64(0)
				if streaming {
					want = 200
				}
				if fixture.Calls()-before != want {
					t.Fatal("backend attempt count changed")
				}
			})
		}
	}
}

// A framed response is available immediately even when a peer leaves its socket
// open. The drain has its own finite bound, then the server still closes.
func TestCompletedResponseDoesNotWaitForASilentPeer(t *testing.T) {
	g := start(t)
	c, err := net.DialTimeout("tcp", g.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	_, err = fmt.Fprintf(c, "GET /v1/models HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nConnection: close\r\n\r\n", g.Addr(), g.Token())
	if err != nil {
		t.Fatal("write request")
	}
	reader := bufio.NewReader(c)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal("read response")
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || !json.Valid(body) || response.StatusCode != 200 {
		t.Fatal("incomplete response")
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatalf("connection did not close cleanly within its bound: %v", err)
	}
}

func TestLargeRefusalPreservesReplyAndConnection(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		t.Run(fmt.Sprint("authorized=", authorized), func(t *testing.T) {
			g := start(t)
			g.EnableContextPolicy()
			c, err := net.DialTimeout("tcp", g.Addr(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			credential := ""
			status := 401
			category := "LOCAL_SESSION_REQUIRED"
			if authorized {
				credential = "Authorization: Bearer " + g.Token() + "\r\n"
				status = 400
				category = "CONTEXT_REQUEST_CLASS_UNVERIFIED"
			}
			body := strings.Repeat("x", 384*1024)
			_, err = fmt.Fprintf(c, "POST /v1/messages HTTP/1.1\r\nHost: %s\r\n%sContent-Type: application/json\r\nAnthropic-Version: %s\r\nContent-Length: %d\r\n\r\n%s", g.Addr(), credential, anthropicVersion, len(body), body)
			if err != nil {
				t.Fatal("request write failed")
			}
			reader := bufio.NewReader(c)
			response, err := http.ReadResponse(reader, nil)
			if err != nil {
				t.Fatal("response header failed", err)
			}
			raw, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil || response.StatusCode != status || !json.Valid(raw) || !strings.Contains(string(raw), category) {
				t.Fatal("refusal body lost", readErr, closeErr, response.StatusCode, len(raw))
			}
			if response.Close {
				t.Fatal("complete bounded upload unnecessarily closed its connection")
			}
			_, err = fmt.Fprintf(c, "GET /v1/models HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n", g.Addr(), g.Token())
			if err != nil {
				t.Fatal("follow-up write failed")
			}
			follow, err := http.ReadResponse(reader, nil)
			if err != nil {
				t.Fatal("same-connection follow-up failed", err)
			}
			raw, readErr = io.ReadAll(follow.Body)
			closeErr = follow.Body.Close()
			if readErr != nil || closeErr != nil || follow.StatusCode != 200 || !json.Valid(raw) {
				t.Fatal("follow-up response incomplete")
			}
		})
	}
}

// Reproduce the V1 parser-refusal failures through the
// Go listener. net/http can reject these before the gateway handler runs.
func TestProtocolRefusalsPreserveResponses(t *testing.T) {
	for _, tc := range []struct {
		name, request string
		status        int
		reuse         bool
	}{
		{"expectation", "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nExpect: public-unsupported\r\nContent-Length: 6\r\nConnection: close\r\n\r\npublic", 417, false},
		{"invalid-header", "GET /v1/models HTTP/1.1\r\nHost: %s\r\nBad Header: public\r\nConnection: close\r\n\r\n", 400, false},
		{"reused-invalid-header", "GET /v1/models HTTP/1.1\r\nHost: %s\r\nBad Header: public\r\nConnection: close\r\n\r\n", 400, true},
		{"transfer-encoding", "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: public-unsupported\r\nConnection: close\r\n\r\n", 501, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for attempt := 0; attempt < 20; attempt++ {
				t.Run(fmt.Sprint(attempt), func(t *testing.T) {
					g := start(t)
					c, err := net.DialTimeout("tcp4", g.Addr(), time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					_ = c.SetDeadline(time.Now().Add(2 * time.Second))
					reader := bufio.NewReader(c)
					if tc.reuse {
						if _, err := fmt.Fprintf(c, "GET /v1/models HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n", g.Addr(), g.Token()); err != nil {
							t.Fatal("initial request write failed")
						}
						first, err := http.ReadResponse(reader, nil)
						if err != nil {
							t.Fatal("initial response lost", err)
						}
						body, readErr := io.ReadAll(first.Body)
						first.Body.Close()
						if readErr != nil || first.StatusCode != 200 || !json.Valid(body) || first.Close {
							t.Fatal("initial response incomplete or connection closed")
						}
					}
					if _, err = fmt.Fprintf(c, tc.request, g.Addr()); err != nil {
						t.Fatal("request write failed", err)
					}
					response, err := http.ReadResponse(reader, nil)
					if err != nil {
						t.Logf("gateway handler requests=%d", g.received.Load())
						t.Fatal("protocol response lost", err)
					}
					defer response.Body.Close()
					// A local HTTP filter may remove an unknown Expect before Go sees it.
					// The unauthenticated request must still be rejected before dispatch.
					if response.StatusCode != tc.status && !(tc.name == "expectation" && response.StatusCode == 401) {
						t.Fatalf("status=%d want=%d", response.StatusCode, tc.status)
					}
					body, err := io.ReadAll(response.Body)
					if err != nil {
						t.Fatal("protocol response truncated", err)
					}
					if response.ContentLength != int64(len(body)) {
						t.Fatalf("response relies on connection close: content-length=%d bytes=%d", response.ContentLength, len(body))
					}
					waitForActive(t, g, 0, "protocol rejection")
				})
			}
		})
	}
}

// A busy or oversized refusal closes the connection instead of first draining the upload,
// so a slow uploader cannot hold the refusal back by the one-second drain budget.
func TestClosingRefusalsDoNotWaitForASlowUpload(t *testing.T) {
	for _, c := range []struct {
		name   string
		sent   int64
		status int
	}{{"busy", 1024, http.StatusTooManyRequests}, {"too-large", maxRequestBytes + 1, http.StatusRequestEntityTooLarge}} {
		t.Run(c.name, func(t *testing.T) {
			g := start(t)
			if c.status == http.StatusTooManyRequests {
				for i := 0; i < maxActiveRequests; i++ {
					_, _, release, err := g.requests.admit(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer release()
				}
			}
			body, upload := io.Pipe()
			defer upload.Close()
			sent := make(chan time.Time, 1)
			started := time.Now()
			go func() { _, _ = io.CopyN(upload, endlessReader{}, c.sent); sent <- time.Now() }() // then the upload stalls
			r, err := http.NewRequest(http.MethodPost, g.BaseURL()+"/v1/messages", body)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range messages(nil).headers {
				r.Header.Set(key, value)
			}
			r.Header.Set("Authorization", "Bearer "+g.Token())
			response, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			answered := time.Now()
			if c.status == http.StatusRequestEntityTooLarge {
				select {
				case started = <-sent:
				case <-time.After(10 * time.Second):
					t.Fatal("the oversized upload never finished")
				}
			}
			if waited := answered.Sub(started); response.StatusCode != c.status || !response.Close || waited > 500*time.Millisecond {
				t.Fatalf("status=%d close=%v after %v", response.StatusCode, response.Close, waited)
			}
		})
	}
}
