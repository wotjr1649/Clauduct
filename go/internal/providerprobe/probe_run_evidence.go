package providerprobe

import (
	"context"
	"errors"
	"os/exec"

	"github.com/wotjr1649/Clauduct/go/internal/app"
)

type p3RunEvidence struct {
	Index                  int                          `json:"index"`
	Gateway                int64                        `json:"gateway"`
	NativeStarted          bool                         `json:"nativeStarted"`
	NativePID              int                          `json:"nativePid,omitempty"`
	NativeCreationFiletime uint64                       `json:"nativeCreationFiletime,string,omitempty"`
	Exit                   int                          `json:"exit"`
	NaturalExit            bool                         `json:"naturalExit"`
	CleanupFailed          bool                         `json:"cleanupFailed"`
	UnexpectedError        bool                         `json:"unexpectedError"`
	Window                 *p3ObservationWindowEvidence `json:"window,omitempty"`
}

func p3BackendTurn(c p3Case, turns []p3Turn, in p3Call) int {
	switch c.Entry {
	case p3ContinueRefusal, p3ContinueRecover, p3ContinueLegacy:
		// Continue runs replay earlier canaries, so their launch generation identifies the turn.
		if in.run <= 0 {
			return -1
		}
		attributed := -1
		for i, turn := range turns {
			if turn.run != in.run {
				continue
			}
			if attributed >= 0 {
				return -1
			}
			attributed = i
		}
		return attributed
	default:
		return turnOf(turns, in.canaries, in.at)
	}
}

func p3UnexpectedRunError(err error, result app.Result, contextErr error) bool {
	if result.Category == app.CategoryRouting || errors.Is(err, app.ErrRoutingUnverified) {
		return true
	}
	return err != nil && !p3OnlyNativeExitError(err) && !p3OnlyContextError(err, contextErr)
}

func p3OnlyNativeExitError(err error) bool {
	switch e := err.(type) {
	case *exec.ExitError:
		return e != nil && e.ProcessState != nil
	case interface{ Unwrap() []error }:
		causes := e.Unwrap()
		return len(causes) == 1 && p3OnlyNativeExitError(causes[0])
	case interface{ Unwrap() error }:
		return p3OnlyNativeExitError(e.Unwrap())
	default:
		return false
	}
}

func p3OnlyContextError(err, contextErr error) bool {
	if contextErr != context.Canceled && contextErr != context.DeadlineExceeded {
		return false
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		causes := e.Unwrap()
		return len(causes) == 1 && p3OnlyContextError(causes[0], contextErr)
	case interface{ Unwrap() error }:
		return p3OnlyContextError(e.Unwrap(), contextErr)
	default:
		return err == contextErr
	}
}
