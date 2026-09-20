package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func BenchmarkContextDisplayFiltering(b *testing.B) {
	blocks := make([]anthropic.Block, 0, 515)
	for range 512 {
		blocks = append(blocks, anthropic.Block{Type: "text", Text: strings.Repeat("public ", 585)})
	}
	key := displayTriple{}
	for i, s := range []string{"command", "rendered", "markdown"} {
		blocks = append(blocks, anthropic.Block{Type: "text", Text: s})
		key[i] = displayHash(s)
	}
	known := map[displayTriple]int{key: 1}
	original := anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: blocks}}}
	b.SetBytes(int64(512 * 585 * 7))
	b.ReportAllocs()
	for b.Loop() {
		req := original
		if removeDisplayTriples(&req, known) != 3 || len(req.Messages[0].Blocks) != 512 {
			b.Fatal("filter lost task input")
		}
	}
}

func TestContextDisplayRequiresNativeProvenance(t *testing.T) {
	const session = "public-session"
	texts := []string{"<command-name>/context</command-name>", "<local-command-stdout>public display</local-command-stdout>", "## Context Usage\npublic report"}
	rows := []map[string]any{
		{"type": "system", "subtype": "local_command", "uuid": "public-command", "content": texts[0]},
		{"type": "system", "subtype": "local_command", "uuid": "public-output", "parentUuid": "public-command", "commandRun": map[string]string{"command": "context", "args": "all"}, "content": texts[1]},
		{"type": "user", "isMeta": true, "parentUuid": "public-output", "message": map[string]string{"role": "user", "content": texts[2]}},
	}
	for _, scenario := range []string{"proven", "unproven", "broken-parent", "wrong-version", "compatible-version", "mixed-version", "quotation", "partial", "compacted", "split-append", "resume"} {
		t.Run(scenario, func(t *testing.T) {
			g, _ := newContextFixture(t)
			dir := t.TempDir()
			g.ConfigureDelegations(dir)
			file := filepath.Join(dir, session+".jsonl")
			if err := g.contextSession(session, file); err != nil {
				t.Fatal(err)
			}
			var raw []byte
			for i, original := range rows {
				row := map[string]any{}
				for k, v := range original {
					row[k] = v
				}
				row["sessionId"], row["version"] = session, ReferenceClient
				if scenario == "broken-parent" && i == 2 {
					row["parentUuid"] = "other"
				}
				if scenario == "wrong-version" {
					row["version"] = "unknown"
				}
				if scenario == "compatible-version" || scenario == "mixed-version" && i == 2 {
					row["version"] = "2.9.1"
				}
				b, _ := json.Marshal(row)
				raw = append(raw, append(b, '\n')...)
			}
			if scenario == "unproven" {
				raw = nil
			}
			if scenario == "compacted" {
				raw = append(raw, []byte("{\"subtype\":\"compact_boundary\"}\n")...)
			}
			makeRequest := func() *anthropic.Request {
				var blocks []anthropic.Block
				for _, text := range texts {
					blocks = append(blocks, anthropic.Block{Type: "text", Text: text})
				}
				if scenario == "quotation" {
					blocks = append(blocks, blocks...)
				}
				if scenario == "partial" {
					blocks = blocks[1:]
				}
				blocks = append(blocks, anthropic.Block{Type: "text", Text: "USER_TASK"})
				return &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: blocks}}}
			}
			if scenario == "split-append" {
				if err := os.WriteFile(file, raw[:len(raw)-3], 0600); err != nil {
					t.Fatal(err)
				}
				r := makeRequest()
				g.stripContextDisplays(r, session)
				if len(r.Messages[0].Blocks) != 4 {
					t.Fatal("partial record authorized removal")
				}
				f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.Write(raw[len(raw)-3:])
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			r := makeRequest()
			original := len(r.Messages[0].Blocks)
			g.stripContextDisplays(r, session)
			want := original
			if scenario == "proven" || scenario == "compatible-version" || scenario == "split-append" || scenario == "resume" {
				want = 1
			}
			if len(r.Messages) != 1 || len(r.Messages[0].Blocks) != want || r.Messages[0].Blocks[want-1].Text != "USER_TASK" {
				t.Fatalf("retained blocks=%v want=%d", r.Messages, want)
			}
			if scenario == "resume" {
				g.displays = displayHistory{}
				r = makeRequest()
				g.stripContextDisplays(r, session)
				if len(r.Messages[0].Blocks) != 1 {
					t.Fatal("resume did not rebuild provenance")
				}
			}
		})
	}
}
