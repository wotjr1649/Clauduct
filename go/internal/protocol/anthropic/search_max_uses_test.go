package anthropic

import (
	"errors"
	"testing"
)

// #272: max_uses caps the searches of one request. This bridge runs one search per side
// query, so a positive cap holds as is; anything else is malformed and refused by name.
func TestSearchMaxUsesMustBeAPositiveInteger(t *testing.T) {
	const head = `{"model":"gpt-6-astra","max_tokens":1,"stream":true,` +
		`"messages":[{"role":"user","content":"x"}],"tools":[{"type":"web_search_20250305","name":"web_search"`
	for value, ok := range map[string]bool{
		``:                             true, // absent
		`,"max_uses":null`:             true,
		`,"max_uses":1`:                true,
		`,"max_uses":8`:                true,
		`,"max_uses":0`:                false,
		`,"max_uses":-1`:               false,
		`,"max_uses":1.5`:              false,
		`,"max_uses":"3"`:              false,
		`,"max_uses":1e3`:              false,
		`,"max_uses":[1]`:              false,
		`,"max_uses":true`:             false,
		`,"max_uses":9007199254740991`: true,
		`,"max_uses":9007199254740992`: false,
	} {
		_, err := DecodeRequest([]byte(head + value + `}]}`))
		if ok {
			if err != nil {
				t.Errorf("%q refused: %v", value, err)
			}
			continue
		}
		var refusal *RequestError
		if !errors.As(err, &refusal) || refusal.Code != CodeUnsupportedTools || refusal.Field != "max_uses" {
			t.Errorf("%q: err = %v, want %s naming max_uses", value, err, CodeUnsupportedTools)
		}
	}
}
