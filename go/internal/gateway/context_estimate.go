package gateway

import "github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"

// Only provider numbers and a scalar text estimate survive between requests.
// This is a previous request's measurement, not the current context size.
type contextUsageAnchor struct {
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	Input        int64  `json:"inputTokens"`
	Output       int64  `json:"outputTokens"`
	TextEstimate int64  `json:"textEstimate"`
}

// A cheap preventive signal, never an exact-count endpoint. Do not tokenize
// base64 images, files or encrypted reasoning as though they were prompt text.
// The second return is any opaque input; the third is media specifically -- image, file and audio
// parts, whose bytes this adds nothing for. They are different questions. Encrypted
// reasoning is opaque too and this build's own replies carry it, so an ordinary multi-turn
// conversation sets the first on every request; a counter built on it would measure
// "there was a conversation" rather than the window it exists to show.
func estimateTextInput(r *bridge.Request) (int64, bool, bool) {
	var size int64
	opaque, media := false, false
	parts := func(items []bridge.InputPart) {
		for _, p := range items {
			switch p.Type {
			case "input_text", "output_text":
				size += int64(len(p.Text)) + 12
			default:
				// Opaque is everything this does not add bytes for. Media is the named
				// subset whose size is the reason a length refusal arrives, spelled out
				// rather than inferred from "not text": a part kind added later would
				// otherwise arrive as media by default and put the counter back to
				// reporting that a conversation happened.
				opaque = true
				media = media || p.Type == "input_image" || p.Type == "input_file" || p.Type == "input_audio"
			}
		}
	}
	for _, e := range r.Input {
		size += 12
		if e.Reasoning != nil {
			opaque = true
			continue
		}
		size += int64(len(e.Arguments) + len(e.Name) + len(e.CallID))
		switch content := e.Content.(type) {
		case string:
			size += int64(len(content))
		case []bridge.InputPart:
			parts(content)
		}
		parts(e.Output)
	}
	for _, tool := range r.Tools {
		size += int64(len(tool.Name)+len(tool.Description)+len(tool.Parameters)) + 60
	}
	if r.Text != nil {
		size += int64(len(r.Text.Format.Schema)+len(r.Text.Format.Name)) + 60
	}
	return (size + 2) / 3, opaque, media
}
