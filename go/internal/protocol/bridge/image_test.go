package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// A one-pixel PNG, the smallest thing that is actually an image.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func imageRequest(t *testing.T, body string) *Request {
	t.Helper()
	request, err := anthropic.DecodeRequest([]byte(body))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	built, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return built
}

// A1. An attached picture reaches the backend as the data URL the baseline sends.
//
// The shape is not approximated. native-protocol.mjs turns {source:{type:'base64',
// media_type, data}} into {type:'input_image', image_url:'data:<media_type>;base64,<data>'}
// and gives it an input entry of its own, and that is what this asserts -- including the
// absence of a text field beside it, which a single struct with omitempty would have added.
func TestAnAttachedImageBecomesTheBackendsDataURL(t *testing.T) {
	built := imageRequest(t, `{"model":"gpt-6-astra","max_tokens":64,"stream":true,"messages":[
	  {"role":"user","content":[
	    {"type":"text","text":"what is this"},
	    {"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+onePixelPNG+`"}}
	  ]}]}`)

	encoded, err := json.Marshal(built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"type":"input_image","image_url":"data:image/png;base64,` + onePixelPNG + `"}`
	if !strings.Contains(string(encoded), want) {
		t.Fatalf("the image did not reach the backend in the baseline's shape.\nwant %s\n got %s",
			want, encoded)
	}
	// An image part carries no text field. A struct that emitted "text":"" beside every
	// picture would be sending a field the baseline does not.
	if strings.Contains(string(encoded), `"image_url":"data:image/png;base64,`+onePixelPNG+`","text"`) ||
		strings.Contains(string(encoded), `"text":"","image_url"`) {
		t.Fatalf("an empty text was written beside the image: %s", encoded)
	}

	// Its own entry, after the text, in the order the client sent them. Two entries:
	// there is no system field here, so there is no developer turn ahead of them.
	if len(built.Input) != 2 {
		t.Fatalf("Input has %d entries, want the text and the image separately: %s",
			len(built.Input), encoded)
	}
	last := built.Input[len(built.Input)-1]
	if last.Role != "user" {
		t.Errorf("the image entry has role %q, want user", last.Role)
	}
	parts, ok := last.Content.([]InputPart)
	if !ok || len(parts) != 1 || parts[0].Type != "input_image" {
		t.Fatalf("the image did not get an entry of its own: %#v", last.Content)
	}
}

// And a picture in a tool result travels with the rest of that result.
func TestAnImageInAToolResultTravelsWithIt(t *testing.T) {
	built := imageRequest(t, `{"model":"gpt-6-astra","max_tokens":64,"stream":true,
	  "tools":[{"name":"Screenshot","description":"d","input_schema":{"type":"object"}}],
	  "messages":[
	    {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Screenshot","input":{}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[
	      {"type":"text","text":"here"},
	      {"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"`+onePixelPNG+`"}}
	    ]}]}]}`)

	encoded, err := json.Marshal(built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"type":"input_image","image_url":"data:image/jpeg;base64,` + onePixelPNG + `"}`
	if !strings.Contains(string(encoded), want) {
		t.Fatalf("a tool result's image did not reach the backend.\nwant %s\n got %s", want, encoded)
	}
}

// The refusals, each named for what is actually wrong with the request.
func TestAMalformedImageIsRefusedByWhatIsWrongWithIt(t *testing.T) {
	for _, c := range []struct {
		name, body, code string
	}{
		{"an unknown key on the block", `{"type":"image","source":{"type":"base64",
		  "media_type":"image/png","data":"` + onePixelPNG + `"},"detail":"high"}`,
			anthropic.CodeImageFields},
		{"an unknown key on the source", `{"type":"image","source":{"type":"base64",
		  "media_type":"image/png","data":"` + onePixelPNG + `","url":"x"}}`,
			anthropic.CodeImageSourceFields},
		{"a url source rather than base64", `{"type":"image","source":{"type":"url",
		  "media_type":"image/png","data":"` + onePixelPNG + `"}}`,
			anthropic.CodeUnsupportedImage},
		{"a media type the backend will not read", `{"type":"image","source":{"type":"base64",
		  "media_type":"image/svg+xml","data":"` + onePixelPNG + `"}}`,
			anthropic.CodeUnsupportedImage},
		{"a payload that is not base64", `{"type":"image","source":{"type":"base64",
		  "media_type":"image/png","data":"not base64!"}}`,
			anthropic.CodeUnsupportedImage},
		{"no source at all", `{"type":"image"}`, anthropic.CodeImageFields},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":64,
			  "stream":true,"messages":[{"role":"user","content":[` + c.body + `]}]}`))
			var refusal *anthropic.RequestError
			if err == nil {
				t.Fatalf("accepted a malformed image")
			}
			if !asRequestError(err, &refusal) || refusal.Code != c.code {
				t.Fatalf("err = %v, want code %s", err, c.code)
			}
		})
	}
}

// An image on an assistant turn is a malformed request, not something to reinterpret.
func TestAnImageOnAnAssistantTurnIsRefused(t *testing.T) {
	_, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":64,"stream":true,
	  "messages":[{"role":"assistant","content":[{"type":"image","source":{"type":"base64",
	  "media_type":"image/png","data":"` + onePixelPNG + `"}}]}]}`))
	var refusal *anthropic.RequestError
	if err == nil || !asRequestError(err, &refusal) || refusal.Code != anthropic.CodeImageRole {
		t.Fatalf("err = %v, want %s", err, anthropic.CodeImageRole)
	}
}

func asRequestError(err error, target **anthropic.RequestError) bool {
	return errors.As(err, target)
}
