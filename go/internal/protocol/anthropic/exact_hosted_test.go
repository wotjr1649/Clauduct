package anthropic

import (
	"strings"
	"testing"
)

func TestHostedAdapterExactVersion319(t *testing.T) {
	for _, version := range []string{"web_search_20990101", "web_search_20260209", "web_search_20999999"} {
		_, err := DecodeRequest([]byte(strings.ReplaceAll(`{"model":"x","max_tokens":20,"messages":[{"role":"user","content":"x"}],"tools":[{"type":"VERSION","name":"web_search"}]}`, "VERSION", version)))
		if err == nil {
			t.Errorf("unregistered version accepted: %s", version)
		}
	}
}
