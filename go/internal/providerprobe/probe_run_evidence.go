package providerprobe

import (
	"context"
	"errors"
	"os/exec"
	"slices"

	"github.com/wotjr1649/Clauduct/go/internal/app"
)

type p3RunEvidence struct {
	Index                  int                          `json:"index"`
	Gateway                int64                        `json:"gateway"`
	NativeStarted          bool                         `json:"nativeStarted"`
	ResumeUUIDRequired     bool                         `json:"resumeUUIDRequired,omitempty"`
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

func p3ContinueRestored(c p3Case, obs *p3Observed, resume int) bool {
	seed := obs.Turns[0].Canary.ID
	current := obs.Turns[resume].Canary.ID
	main := func(in p3Call) bool {
		return in.class == "main" && in.root && in.path == "/v1/messages" && in.gateway > 0 && in.seq > 0
	}
	var firstSeed, firstResume p3Call
	for _, in := range obs.Ingress {
		if !main(in) {
			continue
		}
		switch p3BackendTurn(c, obs.Turns, in) {
		case 0:
			if firstSeed.seq == 0 || in.seq < firstSeed.seq {
				firstSeed = in
			}
		case resume:
			if firstResume.seq == 0 || in.seq < firstResume.seq {
				firstResume = in
			}
		}
	}
	if firstSeed.seq == 0 || firstResume.seq == 0 {
		return false
	}
	correctPair := func(call p3Call) bool {
		return call.backendModel == p3RestoreModel && call.backendEffort == p3RestoreEffort
	}
	dispatch := func(in p3Call) p3Call {
		for _, call := range obs.Calls {
			if call.run == in.run && call.gateway == in.gateway && call.seq == in.seq {
				return call
			}
		}
		return p3Call{}
	}
	start, restored := dispatch(firstSeed), dispatch(firstResume)
	if !correctPair(start) || !slices.Contains(start.canaries, seed) || !correctPair(restored) ||
		!slices.Contains(restored.canaries, seed) || !slices.Contains(restored.canaries, current) {
		return false
	}
	// No later dispatch for this conversation may silently select a different pair.
	for _, in := range obs.Ingress {
		if !main(in) || in.run != firstResume.run || in.gateway != firstResume.gateway || in.seq <= firstResume.seq {
			continue
		}
		call := dispatch(in)
		if (in.backendStarted || call.seq != 0) && (slices.Contains(in.canaries, seed) || slices.Contains(call.canaries, seed)) && !correctPair(call) {
			return false
		}
	}
	return true
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
