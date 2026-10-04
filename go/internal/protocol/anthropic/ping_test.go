package anthropic

import (
	"strings"
	"testing"
)

func frameTypes(frames []Frame) string {
	names := make([]string, len(frames))
	for i, f := range frames {
		names[i] = f.Type
	}
	return strings.Join(names, ",")
}

// A keepalive opens the message only once the backend has named the response, since
// message_start carries the name, and after that it is a bare ping (#120).
func TestPingOpensTheMessageOnlyOnceTheResponseIsNamed(t *testing.T) {
	b := NewBuilder("public-model")
	if got := b.Ping(); got != nil {
		t.Fatalf("ping before the response is named = %s, want nothing", frameTypes(got))
	}
	if err := b.SetResponseID("resp_1"); err != nil {
		t.Fatal(err)
	}
	if got := frameTypes(b.Ping()); got != "message_start,ping" {
		t.Fatalf("first ping = %s", got)
	}
	if got := frameTypes(b.Ping()); got != "ping" {
		t.Fatalf("second ping = %s", got)
	}
	frames, err := b.AppendText("item", 0, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got := frameTypes(frames); got != "content_block_start,content_block_delta" {
		t.Fatalf("text after a ping = %s, want no second message_start", got)
	}
}

// The paths that hold the message back until Complete must not open it a second time when
// a keepalive already did: deferred text, and a parent waiting for its children.
func TestAMessageAKeepaliveOpenedIsNotOpenedAgain(t *testing.T) {
	for name, hold := range map[string]func(*Builder){
		"deferred text":        (*Builder).DeferTextUntilComplete,
		"waiting for children": (*Builder).WaitForChildren,
	} {
		t.Run(name, func(t *testing.T) {
			b := NewBuilder("public-model")
			hold(b)
			_ = b.SetResponseID("resp_1")
			if name == "deferred text" {
				if _, err := b.AppendText("item", 0, "hi"); err != nil {
					t.Fatal(err)
				}
			}
			opened := strings.Count(frameTypes(b.Ping()), "message_start")
			frames, err := b.Complete(Usage{})
			if err != nil {
				t.Fatal(err)
			}
			if total := opened + strings.Count(frameTypes(frames), "message_start"); total != 1 {
				t.Fatalf("message_start sent %d times: ping then %s", total, frameTypes(frames))
			}
			if b.Ping() != nil {
				t.Fatal("a completed response still pings")
			}
		})
	}
}
