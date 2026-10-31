package codex

import (
	"strings"
	"testing"
)

func TestModerationMetadataEnvelope(t *testing.T) {
	const body = `{"type":"response.metadata","response_id":"resp_public","sequence_number":0,"metadata":{"openai_chatgpt_moderation_metadata":{"opaque":{"x":[null,1,"public"]}}}}`
	if ok, err := ModerationMetadata([]byte(body)); !ok || err != nil {
		t.Fatalf("known presentation: %v", err)
	}
	for _, bad := range []string{
		strings.Replace(body, `"sequence_number":0`, `"sequence_number":null`, 1),
		strings.Replace(body, `"sequence_number":0`, `"sequence_number":"0"`, 1),
		strings.Replace(body, `"sequence_number":0`, `"sequence_number":0.0`, 1),
		strings.Replace(body, `"sequence_number":0`, `"sequence_number":9223372036854775808`, 1),
		strings.Replace(body, `"response_id":"resp_public"`, `"response_id":false`, 1),
		strings.Replace(body, `"type":"response.metadata"`, `"type":"response.metadata","type":"response.metadata"`, 1),
		strings.Replace(body, `"type":"response.metadata"`, `"type":"response.created"`, 1),
		body + ` {}`, body[:len(body)-1],
	} {
		if ok, err := ModerationMetadata([]byte(bad)); ok || err == nil {
			t.Errorf("malformed envelope accepted: %s", bad)
		}
	}
}
