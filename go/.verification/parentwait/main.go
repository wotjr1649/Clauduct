// Public, bounded loopback experiment. Production app/gateway/native hooks are
// exercised; only the upstream model is a deterministic fixture. No credentials.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type fixture struct {
	mu                       sync.Mutex
	calls                    int
	steps                    map[string]int
	events                   []map[string]any
	variant                  string
	run, task                string
	stopIssued, resumeIssued bool
	userRead                 bool
	deadlineRelease          chan struct{}
	deadlineOnce             sync.Once
}

func (f *fixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func event(kind string, fields map[string]any) string {
	fields["type"] = kind
	b, _ := json.Marshal(fields)
	return "data: " + string(b) + "\n\n"
}
func answer(c upstream.Call, text string, tool map[string]any) string {
	s := event("response.created", map[string]any{"response": map[string]any{"id": "public-fixture", "model": c.Model}})
	if tool != nil {
		args, _ := json.Marshal(tool["input"])
		s += event("response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": "fc_public", "type": "function_call"}})
		s += event("response.function_call_arguments.delta", map[string]any{"item_id": "fc_public", "delta": string(args)})
		s += event("response.function_call_arguments.done", map[string]any{"item_id": "fc_public", "arguments": string(args)})
		name := "Agent"
		if supplied, ok := tool["name"].(string); ok {
			name = supplied
		}
		s += event("response.output_item.done", map[string]any{"output_index": 0, "item": map[string]any{"id": "fc_public", "type": "function_call", "call_id": tool["id"], "name": name, "arguments": string(args)}})
	} else if text != "ZERO_CONTENT" {
		s += event("response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": "msg_public", "type": "message"}})
		s += event("response.output_text.delta", map[string]any{"item_id": "msg_public", "content_index": 0, "delta": text})
		s += event("response.output_text.done", map[string]any{"item_id": "msg_public", "content_index": 0, "text": text})
		s += event("response.output_item.done", map[string]any{"output_index": 0, "item": map[string]any{"id": "msg_public", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}})
	}
	s += event("response.completed", map[string]any{"response": map[string]any{"id": "public-fixture", "model": c.Model, "usage": map[string]int{"input_tokens": 1000, "output_tokens": 10, "total_tokens": 1010}, "output": []any{}}})
	return s + "data: [DONE]\n\n"
}
func agent(id, model, prompt string) map[string]any {
	return map[string]any{"id": id, "input": map[string]any{"subagent_type": "general-purpose", "description": prompt, "prompt": prompt, "model": model, "effort": "low", "run_in_background": true}}
}
func (f *fixture) Execute(ctx context.Context, c upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	f.calls++
	if f.calls > 22 {
		f.mu.Unlock()
		return nil, errors.New("FIXTURE_LIMIT")
	}
	key := c.Model
	if f.variant == "workflow" && c.Source == "workflow-selection" {
		key = "workflow-" + c.Model
	}
	if strings.HasPrefix(c.Source, "delegation-inherited") {
		key = "leaf"
	}
	if !upstream.Conversation(string(c.Body)) {
		key = "auxiliary"
	}
	f.steps[key]++
	n := f.steps[key]
	body := string(c.Body)
	hasMiddle := strings.Contains(body, "MIDDLE_PRODUCT_WAIT_OK")
	hasLeaf := strings.Contains(body, "LEAF_PRODUCT_WAIT_OK")
	f.events = append(f.events, map[string]any{"model": c.Model, "kind": key, "step": n, "at": time.Now().UTC(), "hasMiddle": hasMiddle, "hasLeaf": hasLeaf})
	f.mu.Unlock()
	if strings.HasPrefix(f.variant, "deadline-") {
		if key != "auxiliary" {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-f.deadlineRelease:
			}
		}
		return &upstream.Response{Body: io.NopCloser(strings.NewReader(answer(c, "PUBLIC_DEADLINE_COMPLETED", nil)))}, nil
	}
	if f.variant == "workflow" {
		return f.workflow(ctx, c, key, n, body)
	}
	if f.variant == "race" && (key == "gpt-6-astra" && n > 1 && !hasMiddle || key == "gpt-5.6-terra" && n > 1 && !hasLeaf) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	text := "PUBLIC_AUXILIARY"
	var tool map[string]any
	switch key {
	case "gpt-6-astra":
		if n == 1 {
			tool = agent("call_public_middle", "gpt-5.6-terra", "PUBLIC_MIDDLE")
		} else if strings.Contains(body, "PUBLIC_AFTER_ESC") {
			text = "PUBLIC_AFTER_ESC_OK"
		} else if strings.Contains(body, "PUBLIC_WHILE_WAITING_WITH_TOOL") && !f.userRead && !hasMiddle {
			f.userRead = true
			tool = map[string]any{"id": "call_public_read", "name": "Read", "input": map[string]any{"file_path": "D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7/project/fixture.txt"}}
		} else if strings.Contains(body, "PUBLIC_WHILE_WAITING") && !hasMiddle {
			text = "PUBLIC_WHILE_WAITING_OK"
		} else if hasMiddle {
			text = "ROOT_PRODUCT_WAIT_OK"
		} else {
			text = "PREMATURE_ROOT_PUBLIC_FINAL"
		}
	case "gpt-5.6-terra":
		if n == 1 {
			tool = agent("call_public_leaf", "gpt-5.6-terra", "PUBLIC_LEAF")
		} else if hasLeaf {
			text = "MIDDLE_PRODUCT_WAIT_OK"
		} else if f.variant == "empty" || f.variant == "race" {
			text = "ZERO_CONTENT"
		} else {
			text = "PREMATURE_MIDDLE_PUBLIC_FINAL"
		}
	case "leaf":
		delay := 12 * time.Second
		if f.variant == "race" {
			delay = 300 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		text = "LEAF_PRODUCT_WAIT_OK"
	}
	return &upstream.Response{Body: io.NopCloser(strings.NewReader(answer(c, text, tool)))}, nil
}
func main() {
	if len(os.Args) != 4 || os.Args[1] != "empty" && os.Args[1] != "text" && os.Args[1] != "workflow" && os.Args[1] != "race" && os.Args[1] != "deadline-stalled" && os.Args[1] != "deadline-completed" {
		panic("ARGS")
	}
	dir := os.Args[2]
	f := &fixture{steps: map[string]int{}, variant: os.Args[1], deadlineRelease: make(chan struct{})}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			env[k] = v
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var deadline time.Duration
	if strings.HasPrefix(f.variant, "deadline-") {
		deadline = 45 * time.Second
	}
	result, err := app.Run(ctx, app.Options{Args: []string{"--session-id", os.Args[3], "--strict-mcp-config"}, Env: env, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		SessionTimeout: deadline, DeadlineGrace: 10 * time.Second,
		ResolveClaude: func() (string, bool, error) { return "C:/Users/js/.local/share/claude/versions/2.1.278", true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(f)
			if e == nil {
				g.EnableContextPolicy()
			}
			return g, e
		},
		Checkpoint: func(status app.Status) error {
			if deadline > 0 {
				f.mu.Lock()
				f.events = append(f.events, map[string]any{"phase": status.Lifecycle.State, "active": status.Gateway.Requests.Active, "at": time.Now().UTC()})
				f.mu.Unlock()
				if f.variant == "deadline-completed" && status.Lifecycle.State == "draining" {
					f.deadlineOnce.Do(func() { close(f.deadlineRelease) })
				}
			}
			raw, e := json.Marshal(status)
			if e != nil {
				return e
			}
			return os.WriteFile(filepath.Join(dir, "status.json"), raw, 0600)
		},
	})
	raw, _ := json.MarshalIndent(map[string]any{"result": result, "error": err != nil, "events": f.events, "fixture": true, "realBackend": false}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "result.json"), raw, 0600)
	if deadline > 0 {
		valid := err == nil && result.Category == app.CategoryDeadline && result.CleanupErr == nil && result.Lifecycle.Final && result.Lifecycle.NativeReaped && result.Lifecycle.GraceExpired == (f.variant == "deadline-stalled") && result.Diagnostics.Requests.Active == 0
		if !valid {
			os.Exit(1)
		}
		return
	}
	if err != nil || result.NativeExitCode != 0 {
		os.Exit(1)
	}
}

func (f *fixture) workflow(ctx context.Context, c upstream.Call, key string, n int, body string) (*upstream.Response, error) {
	text := "PUBLIC_WORKFLOW_WAIT"
	var tool map[string]any
	switch key {
	case "workflow-gpt-5.6-terra":
		text = "PUBLIC_PLAN_A_OK"
	case "workflow-gpt-5.6-sol":
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(60 * time.Second):
		}
		text = "PUBLIC_PLAN_B_OK"
	case "workflow-gpt-5.6-luna":
		text = "PUBLIC_PLAN_C_OK"
	case "gpt-6-astra":
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.run == "" || f.task == "" {
			entries, _ := os.ReadDir(os.Getenv("TEMP"))
			for _, e := range entries {
				if !e.IsDir() || !strings.HasPrefix(e.Name(), "clauduct-native-events-") {
					continue
				}
				files, _ := filepath.Glob(filepath.Join(os.Getenv("TEMP"), e.Name(), "receipts", "workflow-*.json"))
				for _, file := range files {
					raw, _ := os.ReadFile(file)
					var link struct{ Run, Task string }
					if json.Unmarshal(raw, &link) == nil {
						f.run, f.task = link.Run, link.Task
						break
					}
				}
			}
		}
		var req struct {
			Input []struct{ Output json.RawMessage }
		}
		_ = json.Unmarshal(c.Body, &req)
		for _, entry := range req.Input {
			var output string
			if json.Unmarshal(entry.Output, &output) != nil {
				continue
			}
			if f.run == "" {
				if m := regexp.MustCompile(`"runId"\s*:\s*"(wf_[A-Za-z0-9_-]+)"`).FindStringSubmatch(output); m != nil {
					f.run = m[1]
				}
			}
			if f.task == "" {
				if m := regexp.MustCompile(`"taskId"\s*:\s*"([A-Za-z0-9_-]+)"`).FindStringSubmatch(output); m != nil {
					f.task = m[1]
				}
			}
		}
		switch {
		case n == 1:
			steps := []map[string]string{{"id": "A", "prompt": "PUBLIC_PLAN_A", "model": "gpt-5.6-terra", "effort": "low"}, {"id": "B", "prompt": "PUBLIC_PLAN_B", "model": "gpt-5.6-sol", "effort": "low"}, {"id": "C", "prompt": "PUBLIC_PLAN_C", "model": "gpt-5.6-luna", "effort": "low"}}
			tool = map[string]any{"id": "call_public_plan", "name": "Workflow", "input": map[string]any{"script": "clauduct:plan-v1", "args": map[string]any{"steps": steps}}}
		case strings.Contains(body, "PUBLIC_STOP_PLAN") && !f.stopIssued:
			if f.task == "" {
				return nil, errors.New("FIXTURE_TASK_ID_MISSING")
			}
			f.stopIssued = true
			tool = map[string]any{"id": "call_public_stop", "name": "TaskStop", "input": map[string]any{"task_id": f.task}}
		case strings.Contains(body, "PUBLIC_RESUME_PLAN") && !f.resumeIssued:
			if f.run == "" {
				return nil, errors.New("FIXTURE_RUN_ID_MISSING")
			}
			f.resumeIssued = true
			tool = map[string]any{"id": "call_public_plan_resume", "name": "Workflow", "input": map[string]any{"resumeFromRunId": f.run}}
		case f.resumeIssued && strings.Contains(body, "PUBLIC_PLAN_C_OK"):
			text = "PUBLIC_PLAN_CONTINUED_OK"
		case f.stopIssued:
			text = "PUBLIC_PLAN_STOP_OBSERVED"
		}
	}
	return &upstream.Response{Body: io.NopCloser(strings.NewReader(answer(c, text, tool)))}, nil
}
