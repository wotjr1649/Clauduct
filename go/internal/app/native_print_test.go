package app

import (
	"strings"
	"testing"
)

func TestNativePrintPreservesStreamResult(t *testing.T) {
	buildHook(t)
	for _, thought := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "text_and_reasoning"}[thought], func(t *testing.T) {
			reply := textStream("public_print", "PUBLIC_PRINT_RESULT")
			if thought {
				reasoning := frames(
					`{"type":"response.output_item.added","output_index":1,"item":{"id":"rs_public","type":"reasoning"}}`,
					`{"type":"response.output_item.done","output_index":1,"item":{"id":"rs_public","type":"reasoning","summary":[],"encrypted_content":"public-synthetic"}}`,
				)
				reasoning = strings.TrimSuffix(reasoning, "data: [DONE]\n\n")
				reply = strings.Replace(reply, `data: {"type":"response.completed"`, reasoning+`data: {"type":"response.completed"`, 1)
			}
			out := (nativeRun{Args: []string{"-p", "Public print result probe", "--model", "gpt-5.6-luna", "--effort", "low"}, Reply: reply, ContextPolicy: true}).run(t)
			if out.err != nil || out.result.NativeExitCode != 0 || !strings.Contains(out.stdout, "PUBLIC_PRINT_RESULT") {
				t.Fatalf("print result lost: exit=%d stdout_bytes=%d marker=%v", out.result.NativeExitCode, len(out.stdout), strings.Contains(out.stdout, "PUBLIC_PRINT_RESULT"))
			}
		})
	}
}
