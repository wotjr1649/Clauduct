// A minimal MCP stdio server, for measuring that --mcp-config actually works through this
// bridge rather than merely reaching the child's argv.
//
// It lives under testdata so it is not part of the module build. It speaks the subset the
// client needs: initialize, tools/list, tools/call.
//
// The echo tool reports whether an environment variable reached this process. That is the
// measurement that matters: the Node baseline dropped anything matching TOKEN or SECRET
// from the child's environment, which is why MCP servers did not work under it.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if json.Unmarshal(line, &req) != nil {
			continue
		}
		result, answer := handle(req)
		if !answer {
			continue
		}
		reply, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": json.RawMessage(req.ID), "result": result,
		})
		if err != nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "stub: %s\n", req.Method)
		out.Write(reply)
		out.WriteByte('\n')
		out.Flush()
	}
}

func handle(req request) (any, bool) {
	switch req.Method {
	case "initialize":
		// Echo the version the client asked for. A server that names its own would have to
		// know which ones this client accepts, and that is the thing being measured.
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if params.ProtocolVersion == "" {
			params.ProtocolVersion = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": params.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "stub", "version": "0.0.1"},
		}, true

	case "tools/list":
		return map[string]any{"tools": []any{map[string]any{
			"name":        "report",
			"description": "Reports whether the server's environment carried a value.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"name": map[string]any{"type": "string"}},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
		}}}, true

	case "tools/call":
		var params struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		value, present := os.LookupEnv(params.Arguments["name"])
		text := "ABSENT"
		if present {
			text = "PRESENT:" + value
		}
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": text}},
		}, true

	case "ping":
		return map[string]any{}, true
	}
	// Notifications carry no id and want no reply.
	return nil, len(req.ID) > 0 && string(req.ID) != "null"
}
