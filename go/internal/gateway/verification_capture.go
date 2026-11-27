package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const (
	verificationResponseLimit = 1 << 20
	verificationRecordLimit   = 96
)

// A verification run that loses a captured response is not evidence, so every loss has a name and
// VerificationCaptureError reports it. Normal launches never create a capture.
var (
	ErrVerificationCaptureWrite     = errors.New("VERIFICATION_HTTP_CAPTURE_WRITE_FAILED")
	ErrVerificationCaptureCollision = errors.New("VERIFICATION_HTTP_CAPTURE_COLLISION")
	ErrVerificationCaptureTooLarge  = errors.New("VERIFICATION_HTTP_CAPTURE_TOO_LARGE")
	ErrVerificationCaptureLimit     = errors.New("VERIFICATION_HTTP_CAPTURE_LIMIT")
	ErrVerificationCaptureLate      = errors.New("VERIFICATION_HTTP_CAPTURE_LATE_WRITE")
)

// openCapture creates one capture file exclusively; tests replace it to force the failures below.
var openCapture = func(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}

type verificationCapture struct {
	dir     string
	records atomic.Int32
	mu      sync.Mutex
	errs    []error // first few losses; the first is enough to fail the run
}

func (v *verificationCapture) fail(kind error, detail string, cause error) {
	err := fmt.Errorf("%w: %s", kind, detail)
	if cause != nil {
		err = fmt.Errorf("%w: %s: %w", kind, detail, cause)
	}
	v.mu.Lock()
	if len(v.errs) < 8 {
		v.errs = append(v.errs, err)
	}
	v.mu.Unlock()
}

// capturedResponse is shared by the handler's writers and the cleanup that publishes it. Every access
// holds mu. Once sealed the body is final, so a write that arrives later is a lost byte and is reported.
type capturedResponse struct {
	owner    *verificationCapture
	mu       sync.Mutex
	body     bytes.Buffer
	tooLarge bool
	sealed   bool
}

func (c *capturedResponse) append(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		c.owner.fail(ErrVerificationCaptureLate, fmt.Sprintf("%d bytes after publication", len(data)), nil)
		return
	}
	if c.tooLarge || len(data) > verificationResponseLimit-c.body.Len() {
		c.tooLarge = true
		c.body.Reset()
		return
	}
	_, _ = c.body.Write(data)
}

// seal freezes the body and returns a copy of it.
func (c *capturedResponse) seal() (body []byte, tooLarge bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sealed = true
	return bytes.Clone(c.body.Bytes()), c.tooLarge
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

// VerificationCaptureError is nil unless a capture was configured and lost a response. It also fails when
// the capture directory itself is gone, since nothing can then have been published.
func (g *Gateway) VerificationCaptureError() error {
	capture := g.verificationCapture.Load()
	if capture == nil {
		return nil
	}
	if info, err := os.Stat(capture.dir); err != nil || !info.IsDir() {
		capture.fail(ErrVerificationCaptureWrite, "capture directory is missing", err)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return errors.Join(capture.errs...)
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
	if capture.records.Add(1) > verificationRecordLimit {
		capture.fail(ErrVerificationCaptureLimit, r.URL.Path, nil)
		return func() {}
	}
	response := &capturedResponse{owner: capture}
	t.capture = response
	session := r.Header.Get("X-Claude-Code-Session-Id")
	if !IsSessionUUID(session) {
		session = ""
	}
	return func() {
		observed := t.rec.snapshot()
		body, tooLarge := response.seal()
		if observed.Status == 0 {
			return
		}
		name := fmt.Sprintf("response-%010d.json", observed.Seq)
		if tooLarge {
			capture.fail(ErrVerificationCaptureTooLarge, name, nil)
			return
		}
		file, err := openCapture(filepath.Join(capture.dir, name))
		if errors.Is(err, fs.ErrExist) {
			capture.fail(ErrVerificationCaptureCollision, name, err)
			return
		}
		if err != nil {
			capture.fail(ErrVerificationCaptureWrite, name, err)
			return
		}
		err = json.NewEncoder(file).Encode(map[string]any{
			"seq": observed.Seq, "method": observed.Method, "path": observed.Path,
			"status": observed.Status, "contentType": t.Header().Get("Content-Type"),
			"nativePid": id.native.pid, "nativeCreationUnixNano": fmt.Sprint(id.native.created),
			"peer": r.RemoteAddr, "listener": local.String(), "session": session,
			"launcherPid": os.Getpid(), "bodyBase64": body,
		})
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			capture.fail(ErrVerificationCaptureWrite, name, err)
		}
	}
}
