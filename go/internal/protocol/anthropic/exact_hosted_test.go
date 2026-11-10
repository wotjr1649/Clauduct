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

func TestHostedSearchSemanticFields319(t *testing.T) {
	for _, field := range []string{
		`"allowed_callers":["future_executor"]`, `"allowed_callers":[]`,
		`"allowed_callers":["assistant","future_executor"]`, `"allowed_callers":null`,
		`"response_inclusion":"future_content"`, `"response_inclusion":null`,
		`"cache_control":{"type":"ephemeral","future_semantic":true}`,
		`"cache_control":{"type":"persistent"}`, `"cache_control":null`,
	} {
		t.Run(field, func(t *testing.T) {
			body := `{"model":"x","max_tokens":20,"messages":[{"role":"user","content":"x"}],"tools":[{"type":"web_search_20250305","name":"web_search",` + field + `}]}`
			if _, err := DecodeRequest([]byte(body)); err == nil {
				t.Fatal("hosted semantic silently accepted")
			}
		})
	}
	for _, fields := range []string{``, `,"allowed_callers":["assistant"],"response_inclusion":"citations","cache_control":{"type":"ephemeral","ttl":"5m"}`} {
		body := `{"model":"x","max_tokens":20,"messages":[{"role":"user","content":"x"}],"tools":[{"type":"web_search_20250305","name":"web_search"` + fields + `}]}`
		if req, err := DecodeRequest([]byte(body)); err != nil || req.HostedSearch == nil {
			t.Fatalf("normal adapter: %v", err)
		}
	}
}

func TestOrdinaryToolNameIsNotHostedSemantic319(t *testing.T) {
	for _, name := range []string{"TotallyNewNativeTool_2099", "web_search", "web_search_20990101"} {
		body := `{"model":"x","max_tokens":20,"messages":[{"role":"user","content":"x"}],"tools":[{"name":"` + name + `","description":"future local callable","input_schema":{"type":"object","properties":{}}}]}`
		req, err := DecodeRequest([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if req.HostedSearch != nil || len(req.Tools) != 1 || req.Tools[0].Name != name {
			t.Fatal("name changed ordinary tool disposition")
		}
	}
}
