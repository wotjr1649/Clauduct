package providercert

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EnvNamespaceSignal names the change-signal file the harness's native-root namespace watch writes
// (INVALID_NATIVE_CHANGED) when it sees a create, rename, delete or overflow in the native root.
// Go treats it as the ADDRESS of a signal only: a raised signal blocks the link (early abort, checked
// at acquisition and immediately before a result is returned), but its absence proves nothing, and
// Go never concludes from the variable that a watch ran. Whether the names were stable for a step is
// the trusted parent harness's final proof (a drained, quiet namespace record covering the step);
// this package only emits evidence for it (NativeImage.PostCheck).
const EnvNamespaceSignal = "CLAUDUCT_NATIVE_STOP_SIGNAL"

// Post-hash check outcomes recorded on every link (NativeImage.PostCheck).
const (
	// PostCheckAliveVerified: after the hash, while the process was still alive, the name it maps then
	// reached the held file object (and the held file was still a file).
	PostCheckAliveVerified = "alive-verified"
	// PostCheckExitedBeforeCheck: the process ended before that check could be made. The acquisition
	// checks passed, so this is a candidate link, and only a final judge that also has a drained,
	// quiet namespace record covering the step may accept it.
	PostCheckExitedBeforeCheck = "exited-before-check"
)

// namespaceCheck is the early abort: a raised signal means no image is linked. No file, or no
// signal yet, is nil and means nothing.
func namespaceCheck() error {
	path := os.Getenv(EnvNamespaceSignal)
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return inconclusive("the namespace change signal is unreadable: %v", err)
	}
	var s struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(raw, &s) != nil || s.Status == "" {
		return inconclusive("the namespace change signal is unreadable")
	}
	if strings.Contains(s.Status, "INVALID_NATIVE_CHANGED") {
		return fmt.Errorf("%w: the namespace watch raised the change signal: %s", ErrNativeChanged, s.Status)
	}
	return inconclusive("the namespace watch raised a signal: %s", s.Status)
}
