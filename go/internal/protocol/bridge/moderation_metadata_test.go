package bridge

import (
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

func TestModerationPresentationMetadataDoesNotAbortOrDeliverContent(t *testing.T) {
	plain := bufferingStream()
	_, want, err := playStream(t, plain)
	if err != nil {
		t.Fatal(err)
	}
	notice := event(codex.MetadataEvent, `{"type":"response.metadata","response_id":"resp_public","metadata":{"openai_chatgpt_moderation_metadata":{"presentation":"PUBLIC_OPAQUE_SENTINEL"}}}`)
	events := append([]stream.Event{plain[0], notice}, plain[1:]...)
	tr, got, err := playStream(t, events)
	if err != nil || got != want || strings.Contains(got, "PUBLIC_OPAQUE_SENTINEL") || tr.ModerationMetadata() != 1 {
		t.Fatalf("presentation interrupted or changed answer: err=%v same=%v leaked=%v", err, got == want, strings.Contains(got, "PUBLIC_OPAQUE_SENTINEL"))
	}
}

func TestModerationPresentationDoesNotAcceptUnknownMetadataOrSafetySignals(t *testing.T) {
	for _, raw := range []string{
		`{"type":"response.metadata","metadata":{"other":{}}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{},"other":{}}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}},"other":1}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}},"safety_buffering":true}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":null}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":[]}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":"x"}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{},"openai_chatgpt_moderation_metadata":{}}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{"a":1,"a":2}}}`,
		`{"type":"response.metadata","metadata":{},"metadata":{"openai_chatgpt_moderation_metadata":{}}}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}},"response_id":null}`,
		`{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}},"sequence_number":-1}`,
	} {
		if _, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, raw)}); err == nil {
			t.Errorf("unread metadata accepted: %s", raw)
		}
	}
	// A presentation notice does not replace a terminal event or a model answer.
	tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6.1-sol")
	frames, err := tr.Accept(event(codex.MetadataEvent, `{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}}}`))
	if err != nil || len(frames) != 0 || tr.Answer() != "" {
		t.Fatalf("notice delivered content: %v", err)
	}
	if _, err := tr.Accept(event(codex.Failed, `{"type":"response.failed","response":{"error":{"code":"server_error"}}}`)); err == nil || errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("notice hid terminal failure: %v", err)
	}
}

func TestModerationPresentationKeepsExpectedRoute(t *testing.T) {
	for _, returned := range []string{
		`"model":"gpt-6-luna","reasoning":{"effort":"high"},`,
		`"model":"gpt-6.1-sol","reasoning":{"effort":"low"},`,
	} {
		tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6.1-sol")
		tr.ExpectRoute("gpt-6.1-sol", "high")
		if _, err := tr.Accept(event(codex.MetadataEvent, `{"type":"response.metadata","metadata":{"openai_chatgpt_moderation_metadata":{}}}`)); err != nil {
			t.Fatal(err)
		}
		frames, err := tr.Accept(event(codex.Completed, `{"type":"response.completed","response":{"id":"resp_public",`+returned+usageReported+`}}`))
		if !errors.Is(err, ErrModelEffortMismatch) || len(frames) != 0 {
			t.Fatalf("notice hid route mismatch: %v", err)
		}
	}
}
