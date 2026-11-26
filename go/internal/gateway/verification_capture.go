package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
)

const verificationResponseLimit = 1 << 20

type verificationCapture struct {
	dir     string
	records atomic.Int32
}

type capturedResponse struct {
	body     bytes.Buffer
	tooLarge bool
}

func (c *capturedResponse) append(data []byte) {
	if c.tooLarge || len(data) > verificationResponseLimit-c.body.Len() {
		c.tooLarge = true
		c.body.Reset()
		return
	}
	_, _ = c.body.Write(data)
}

// CaptureNativeVerificationResponses records bounded responses from authenticated,
// OS-proven native connections. Request headers and bodies are excluded.
func (g *Gateway) CaptureNativeVerificationResponses(budget string) error {
	dir := filepath.Join(budget, "http-capture")
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	g.verificationCapture.Store(&verificationCapture{dir: dir})
	return nil
}

func (g *Gateway) observeVerificationResponse(w http.ResponseWriter, r *http.Request) func() {
	capture := g.verificationCapture.Load()
	if capture == nil {
		return func() {}
	}
	switch r.URL.Path {
	case "/v1/models", "/v1/messages", "/v1/messages/count_tokens":
	default:
		return func() {}
	}
	t, ok := w.(*tracked)
	id, required := g.identityOf(r)
	local, hasLocal := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || !required || !id.direct || id.native.pid <= 0 || id.native.created <= 0 || !hasLocal {
		return func() {}
	}
	if capture.records.Add(1) > 96 {
		return func() {}
	}
	t.capture = &capturedResponse{}
	session := r.Header.Get("X-Claude-Code-Session-Id")
	if !IsSessionUUID(session) {
		session = ""
	}
	return func() {
		observed := t.rec.snapshot()
		if t.capture.tooLarge || observed.Status == 0 {
			return
		}
		name := filepath.Join(capture.dir, fmt.Sprintf("response-%010d.json", observed.Seq))
		file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		defer file.Close()
		_ = json.NewEncoder(file).Encode(map[string]any{
			"seq": observed.Seq, "method": observed.Method, "path": observed.Path,
			"status": observed.Status, "contentType": t.Header().Get("Content-Type"),
			"nativePid": id.native.pid, "nativeCreationUnixNano": fmt.Sprint(id.native.created),
			"peer": r.RemoteAddr, "listener": local.String(), "session": session,
			"launcherPid": os.Getpid(), "bodyBase64": t.capture.body.Bytes(),
		})
	}
}
