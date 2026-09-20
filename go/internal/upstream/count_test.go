package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBackendCountRequiresCompletedUsageWithoutGeneration(t *testing.T) {
	for name, body := range map[string]string{
		"valid":          `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":21,"output_tokens":0}}}`,
		"missing":        `{"type":"response.completed","response":{"output":[],"usage":{"output_tokens":0}}}`,
		"zero":           `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0}}}`,
		"negative":       `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":-1,"output_tokens":0}}}`,
		"generated":      `{"type":"response.completed","response":{"output":[{}],"usage":{"input_tokens":21,"output_tokens":0}}}`,
		"output-tokens":  `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":21,"output_tokens":1}}}`,
		"output-unknown": `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":21}}}`,
		"failed":         `{"type":"response.failed"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var safe atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				ctx, cancel := context.WithTimeout(r.Context(), time.Second)
				defer cancel()
				_, data, err := conn.Read(ctx)
				var sent map[string]json.RawMessage
				if err != nil || json.Unmarshal(data, &sent) != nil {
					return
				}
				safe.Store(string(sent["generate"]) == "false" && string(sent["store"]) == "false" && sent["stream"] == nil && string(sent["type"]) == `"response.create"`)
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.metadata"}`))
				_ = conn.Write(ctx, websocket.MessageText, []byte(body))
				_, _, _ = conn.Read(ctx)
			}))
			defer server.Close()
			d := direct(t, nil, credentialStore(t, false), approved())
			d.endpoint = server.URL
			count, err := d.Count(context.Background(), call(routedBody))
			if name == "valid" {
				if err != nil || count != 21 {
					t.Fatalf("count=%d err=%v", count, err)
				}
			} else if !errors.Is(err, ErrCountFailed) {
				t.Fatalf("invalid count accepted: %d %v", count, err)
			}
			if !safe.Load() {
				t.Fatal("count could generate or persist a response")
			}
			attempts, inferences, _ := d.Ledger.Spent()
			if attempts != 1 || inferences != 0 {
				t.Fatalf("attempts=%d inferences=%d", attempts, inferences)
			}
		})
	}
}

func TestBackendCountReusesOnlyCompletedExclusiveConnections(t *testing.T) {
	var sockets atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		sockets.Add(1)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var payload struct {
				Input    string `json:"input"`
				Generate bool   `json:"generate"`
				Store    bool   `json:"store"`
				Previous string `json:"previous_response_id"`
			}
			if json.Unmarshal(data, &payload) != nil || payload.Generate || payload.Store || payload.Previous != "" {
				return
			}
			if payload.Input == "fail" {
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.failed"}`))
				continue
			}
			_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":%d,"output_tokens":0}}}`, len(payload.Input)+20)))
		}
	}))
	defer server.Close()
	d := direct(t, nil, credentialStore(t, false), Unlimited())
	d.endpoint = server.URL
	defer d.Close()
	count := func(input string) (int64, error) {
		body := fmt.Sprintf(`{"model":"gpt-5.6-luna","reasoning":{"effort":"low"},"input":%q}`, input)
		return d.Count(context.Background(), call(body))
	}
	for _, input := range []string{"one", "another exact input", "small"} {
		n, err := count(input)
		if err != nil || n != int64(20+len(input)) {
			t.Fatalf("count=%d error=%v", n, err)
		}
	}
	if sockets.Load() != 1 {
		t.Fatalf("sequential counts opened %d sockets", sockets.Load())
	}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := fmt.Sprintf("parallel-%d", i)
			n, err := count(input)
			if err != nil || n != int64(20+len(input)) {
				t.Errorf("parallel count=%d error=%v", n, err)
			}
		}()
	}
	wg.Wait()
	if _, err := count("fail"); !errors.Is(err, ErrCountFailed) {
		t.Fatal("failure was not propagated")
	}
	if n, err := count("after failure"); err != nil || n != 33 {
		t.Fatalf("next independent request: %d %v", n, err)
	}
	attempts, inferences, _ := d.Ledger.Spent()
	if attempts != 45 || inferences != 0 {
		t.Fatalf("attempts=%d inferences=%d", attempts, inferences)
	}
}

func TestBackendCountCannotBypassBudgetCredentialsOrRedirectPolicy(t *testing.T) {
	for _, synthetic := range []bool{true, false} {
		server := serve(t, &listener{status: http.StatusFound, headers: http.Header{"Location": []string{"https://invalid.example/never"}}})
		d := direct(t, server, credentialStore(t, synthetic), approved())
		_, err := d.Count(context.Background(), call(routedBody))
		if synthetic {
			if !errors.Is(err, ErrSyntheticMixing) || server.hits.Load() != 0 {
				t.Fatal("synthetic credential sent")
			}
		} else if !errors.Is(err, ErrRedirected) || server.hits.Load() != 1 {
			t.Fatalf("redirect boundary err=%v hits=%d", err, server.hits.Load())
		}
	}
	d := direct(t, nil, credentialStore(t, true), Budget{})
	if _, err := d.Count(context.Background(), call(routedBody)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatal("count bypassed budget")
	}
}
