// Public native TUI fault injection. The loopback proxy intentionally retains
// upstream requests after native disconnects, reproducing a filter's lost abort.
// No subscription, credentials, global settings, or arbitrary file access.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type fixture struct {
	mu     sync.Mutex
	events []map[string]any
	calls  int
}

func (f *fixture) note(kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, map[string]any{"kind": kind, "at": time.Now().UTC()})
}
func event(w io.Writer, kind string, value map[string]any) error {
	value["type"] = kind
	raw, _ := json.Marshal(value)
	_, err := io.WriteString(w, "data: "+string(raw)+"\n\n")
	return err
}
func (f *fixture) Execute(ctx context.Context, c upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if n > 12 {
		return nil, errors.New("FIXTURE_CAP")
	}
	r, w := io.Pipe()
	go func() {
		defer w.Close()
		if event(w, "response.created", map[string]any{"response": map[string]any{"id": "s47_public", "model": c.Model}}) != nil {
			return
		}
		if upstream.Conversation(string(c.Body)) && !strings.Contains(string(c.Body), "PUBLIC_AFTER_ESC") {
			f.note("partial_tool_started")
			if event(w, "response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": "fc_public", "type": "function_call"}}) != nil {
				return
			}
			delta := `{"file_path":"must-not-exist-s47.txt","content":"`
			for {
				if event(w, "response.function_call_arguments.delta", map[string]any{"item_id": "fc_public", "delta": delta}) != nil {
					return
				}
				delta = "public "
				select {
				case <-ctx.Done():
					f.note("upstream_cancelled")
					w.CloseWithError(ctx.Err())
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		text := "PUBLIC_AUXILIARY"
		if upstream.Conversation(string(c.Body)) {
			text = "PUBLIC_AFTER_ESC_OK"
			f.note("recovery_answer")
		}
		event(w, "response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": "msg_public", "type": "message"}})
		event(w, "response.output_text.delta", map[string]any{"item_id": "msg_public", "content_index": 0, "delta": text})
		event(w, "response.output_text.done", map[string]any{"item_id": "msg_public", "content_index": 0, "text": text})
		event(w, "response.output_item.done", map[string]any{"output_index": 0, "item": map[string]any{"id": "msg_public", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}})
		event(w, "response.completed", map[string]any{"response": map[string]any{"id": "s47_public", "model": c.Model, "usage": map[string]int{"input_tokens": 1000, "output_tokens": 10, "total_tokens": 1010}, "output": []any{}}})
	}()
	return &upstream.Response{Body: r}, nil
}

func main() {
	if len(os.Args) != 4 || os.Args[1] != "filtercancel" {
		panic("ARGS")
	}
	dir, id := os.Args[2], os.Args[3]
	f := &fixture{}
	var proxy *http.Server
	var endpoint string
	env := map[string]string{}
	for _, entry := range os.Environ() {
		k, v, ok := strings.Cut(entry, "=")
		if ok {
			env[k] = v
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	result, err := app.Run(ctx, app.Options{Args: []string{"--session-id", id, "--strict-mcp-config"}, Env: env, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		ResolveClaude: func() (string, bool, error) { return "C:/Users/js/.local/share/claude/versions/2.1.278", true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(f)
			if e != nil {
				return nil, e
			}
			g.EnableContextPolicy()
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				return g, e
			}
			endpoint = "http://" + l.Addr().String()
			transport := &http.Transport{Proxy: nil}
			client := &http.Client{Transport: transport}
			proxy = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
				if e != nil {
					w.WriteHeader(400)
					return
				}
				// Deliberate fault: do not propagate r.Context cancellation.
				bounded, stop := context.WithTimeout(context.Background(), 90*time.Second)
				defer stop()
				out, e := http.NewRequestWithContext(bounded, r.Method, g.BaseURL()+r.URL.RequestURI(), bytes.NewReader(body))
				if e != nil {
					w.WriteHeader(500)
					return
				}
				out.Header = r.Header.Clone()
				finished := context.AfterFunc(r.Context(), func() { f.note("proxy_client_closed") })
				defer finished()
				response, e := client.Do(out)
				if e != nil {
					f.note("proxy_request_failed")
					w.WriteHeader(502)
					return
				}
				defer response.Body.Close()
				for k, values := range response.Header {
					for _, v := range values {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(response.StatusCode)
				buffer := make([]byte, 4096)
				for {
					n, e := response.Body.Read(buffer)
					if n > 0 {
						w.Write(buffer[:n])
						http.NewResponseController(w).Flush()
					}
					if e != nil {
						break
					}
				}
			})}
			go proxy.Serve(l)
			return g, nil
		},
		StartProcess: func(spec launch.Spec, in io.Reader, out, errout io.Writer) (app.Process, error) {
			for i, v := range spec.Env {
				if strings.HasPrefix(v, "ANTHROPIC_BASE_URL=") {
					spec.Env[i] = "ANTHROPIC_BASE_URL=" + endpoint
				}
			}
			cmd := exec.Command(spec.File, spec.Args...)
			cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Dir, spec.Env, in, out, errout
			return childprocess.Start(cmd)
		},
		Checkpoint: func(s app.Status) error {
			raw, e := json.Marshal(s)
			if e != nil {
				return e
			}
			return os.WriteFile(filepath.Join(dir, "status.json"), raw, 0600)
		},
	})
	if proxy != nil {
		proxy.Close()
	}
	f.mu.Lock()
	events := append([]map[string]any(nil), f.events...)
	f.mu.Unlock()
	raw, _ := json.MarshalIndent(map[string]any{"result": result, "error": err != nil, "events": events, "fixture": true, "realBackend": false}, "", "  ")
	os.WriteFile(filepath.Join(dir, "result.json"), raw, 0600)
	if err != nil || result.NativeExitCode != 0 {
		os.Exit(1)
	}
}
