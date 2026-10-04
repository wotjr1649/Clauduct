package bridge

import (
	"errors"
	"strings"
	"testing"
)

// The catalogue is product data (models.json); the embedded document must parse, and a
// malformed one must be refused rather than routed on in part.
func TestModelCatalogueIsCheckedProductData(t *testing.T) {
	c, err := parseCatalogue(catalogueDocument)
	if err != nil || len(c.Models) != len(LegacyModels()) || len(c.Retired) != len(Retired) {
		t.Fatalf("embedded catalogue: %v", err)
	}
	valid := `{"version":1,"models":[{"key":"sol","id":"gpt-6.1-sol","alias":"opus","family":"claude-opus-","countValidated":true},{"key":"sol6","id":"gpt-6-sol","agentAlias":"opus"}],"retired":{"gpt-5.6-sol":"gpt-6-sol"},"retiredRoles":{"keys":["sol"],"efforts":["low"]}}`
	if _, err := parseCatalogue([]byte(valid)); err != nil {
		t.Fatalf("valid document refused: %v", err)
	}
	for name, broken := range map[string]string{
		"version":            strings.Replace(valid, `"version":1`, `"version":2`, 1),
		"unknown field":      strings.Replace(valid, `"countValidated":true`, `"countValidated":true,"price":1`, 1),
		"no agent tier":      strings.Replace(valid, `,"agentAlias":"opus"`, ``, 1),
		"model efforts":      strings.Replace(valid, `"countValidated":true`, `"countValidated":true,"efforts":["low"]`, 1),
		"unknown tier":       strings.Replace(valid, `"agentAlias":"opus"`, `"agentAlias":"sonnet"`, 1),
		"tier beside alias":  strings.Replace(valid, `"alias":"opus",`, `"alias":"opus","agentAlias":"opus",`, 1),
		"alias no family":    strings.Replace(valid, `,"family":"claude-opus-"`, ``, 1),
		"duplicate name":     strings.Replace(valid, `"key":"sol6"`, `"key":"sol"`, 1),
		"retired is live":    strings.Replace(valid, `"gpt-5.6-sol":"gpt-6-sol"`, `"gpt-6-sol":"gpt-6.1-sol"`, 1),
		"retired nowhere":    strings.Replace(valid, `"gpt-5.6-sol":"gpt-6-sol"`, `"gpt-5.6-sol":"gpt-7-sol"`, 1),
		"retired effort":     strings.Replace(valid, `"efforts":["low"]}}`, `"efforts":["ultra"]}}`, 1),
		"bad id":             strings.Replace(valid, `"id":"gpt-6-sol"`, `"id":"GPT 6"`, 1),
		"trailing data":      valid + `{}`,
		"trailing brace":     valid + `}`,
		"trailing bracket":   valid + `]`,
		"overlapping family": strings.Replace(valid, `{"key":"sol6","id":"gpt-6-sol","agentAlias":"opus"}`, `{"key":"sol6","id":"gpt-6-sol","efforts":["low"],"alias":"sonnet","family":"claude-opus-5"}`, 1),
	} {
		if _, err := parseCatalogue([]byte(broken)); !errors.Is(err, errCatalogue) {
			t.Fatalf("%s accepted", name)
		}
	}
}
