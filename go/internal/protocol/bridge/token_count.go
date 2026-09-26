package bridge

import (
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/tiktoken-go/tokenizer/codec"
)

var ErrTokenCountUnsupported = errors.New("COUNT_TOKENS_UNSUPPORTED")
var textTokenizer = sync.OnceValue(codec.NewO200kBase)

// CountInput covers the plain-text Responses shape validated against subscription
// backend usage. Tools, media, encrypted reasoning and structured output have
// different framing and must never inherit this formula by approximation.
func CountInput(request *Request) (int64, error) {
	if request == nil {
		return 0, ErrTokenCountUnsupported
	}
	known := false
	for _, model := range Models {
		if request.Model == model.ID && model.CountValidated {
			known = true
			break
		}
	}
	if !known || len(request.Tools) != 0 || request.Text != nil || len(request.Input) == 0 || len(request.Input) > 256 {
		return 0, ErrTokenCountUnsupported
	}
	// The request itself contributes 1 token. A plain developer string adds 4 framing
	// tokens; a text-part message adds 5. Multi-part text is newline-joined. The framing
	// matched 64 live cells across all four catalogue models; the base was 13 while a fixed
	// instructions string was sent and measured 1 without it (#144, 16 cells, 2026-09-26).
	total := int64(1)
	bytesLeft := 4 << 20
	texts := make([]string, 0, len(request.Input))
	wantRole := "user"
	for index, entry := range request.Input {
		if entry.Type != "" || entry.Reasoning != nil || (entry.Role != "developer" && entry.Role != "user" && entry.Role != "assistant") {
			return 0, ErrTokenCountUnsupported
		}
		var text string
		framing := int64(5)
		switch content := entry.Content.(type) {
		case string:
			if entry.Role != "developer" || index != 0 || content == "" {
				return 0, ErrTokenCountUnsupported
			}
			text = content
			framing = 4
		case []InputPart:
			if entry.Role != wantRole {
				return 0, ErrTokenCountUnsupported
			}
			if wantRole == "user" {
				wantRole = "assistant"
			} else {
				wantRole = "user"
			}
			if len(content) == 0 || len(content) > 256 {
				return 0, ErrTokenCountUnsupported
			}
			parts := make([]string, len(content))
			for i, part := range content {
				if part.Type != "input_text" && part.Type != "output_text" {
					return 0, ErrTokenCountUnsupported
				}
				parts[i] = part.Text
			}
			text = strings.Join(parts, "\n")
		default:
			return 0, ErrTokenCountUnsupported
		}
		bytesLeft -= len(text)
		if bytesLeft < 0 || !countableText(text) {
			return 0, ErrTokenCountUnsupported
		}
		texts = append(texts, text)
		total += framing
	}
	if wantRole != "assistant" {
		return 0, ErrTokenCountUnsupported
	}
	for _, text := range texts {
		tokens, err := textTokenizer().Count(text)
		if err != nil {
			return 0, errors.New("COUNT_TOKENS_FAILED")
		}
		total += int64(tokens)
	}
	return total, nil
}

// BackendCountSupported is the paired warmup/generation validation scope.
func BackendCountSupported(request *Request) bool {
	if request == nil {
		return false
	}
	known := false
	for _, model := range Models {
		known = known || request.Model == model.ID && model.CountValidated
	}
	if !known {
		return false
	}
	for _, tool := range request.Tools {
		if tool.Type != "function" {
			return false
		}
	}
	partsOK := func(parts []InputPart) bool {
		for _, part := range parts {
			if part.Type != "input_text" && part.Type != "output_text" && part.Type != "input_image" && part.Type != "input_file" {
				return false
			}
		}
		return true
	}
	for _, entry := range request.Input {
		if entry.Reasoning != nil {
			continue
		}
		switch entry.Type {
		case "function_call":
		case "function_call_output":
			if !partsOK(entry.Output) {
				return false
			}
			// S45: identical JPEG tool outputs undercounted by 1,418 tokens in
			// warmup while generation understood the images. Not an exact oracle.
			for _, part := range entry.Output {
				if part.Type == "input_image" || part.Type == "input_file" {
					return false
				}
			}
		case "":
			switch content := entry.Content.(type) {
			case string:
			case []InputPart:
				if !partsOK(content) {
					return false
				}
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Bound the tokenizer's quadratic merge-pair work on long unbroken pieces. A
// request outside this range gets an explicit unsupported response, not a guess.
func countableText(text string) bool {
	if !utf8.ValidString(text) || strings.Contains(text, "<|") {
		return false
	}
	run := 0
	wasSpace := false
	for _, r := range text {
		space := unicode.IsSpace(r)
		if space != wasSpace {
			run = 0
			wasSpace = space
		}
		run += utf8.RuneLen(r)
		if run > 512 {
			return false
		}
	}
	return true
}
