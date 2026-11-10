package providerprobe

import (
	"slices"

	"github.com/wotjr1649/Clauduct/go/internal/app"
)

type p3R03Request struct {
	Run                                              int
	Gateway, Seq                                     int64
	Turn, Method, Path, Category, ConfirmationSHA256 string
	Status                                           int
	VariantVerified                                  bool
}

type p3R03Evidence struct {
	Complete        bool
	VariantVerified bool
	Run             p3RunEvidence
	Requests        []p3R03Request
	Truncated       bool
}

func p3R03Case(c p3Case) bool {
	return slices.Contains([]string{"R03ps", "R03pf", "R03px"}, c.ID)
}

func p3R03Main(in p3Call) bool {
	return in.class == "main" && in.root && in.path == "/v1/messages"
}

// A refusal is evidence only after every retry and the native's natural, reaped exit.
func p3R03EvidenceOf(c p3Case, observed *p3Observed) p3R03Evidence {
	e := p3R03Evidence{}
	if len(observed.Runs) != 1 {
		return e
	}
	e.VariantVerified = true
	e.Run = observed.Runs[0]
	e.Complete = e.Run.Index > 0 && e.Run.Gateway > 0 && e.Run.NativeStarted && e.Run.Exit != app.ExitCodeUnknown && len(observed.ExitCodes) == 1 && observed.ExitCodes[0] == e.Run.Exit &&
		!e.Run.CleanupFailed && !e.Run.UnexpectedError && observed.NativeStarts == 1 &&
		!observed.TimedOut && !observed.CleanupFailed && !observed.SpawnFailed && !observed.UnexpectedRunError && observed.ReceiversDrained && !observed.ReceiverFault
	for _, in := range observed.Ingress {
		if !p3R03Main(in) {
			continue
		}
		turn := "turn1"
		if !slices.Contains(in.canaries, "turn1") {
			e.Complete = false
			turn = ""
		}
		if len(e.Requests) == p3VariantMaxReceipts {
			e.Truncated = true
			e.Complete = false
			break
		}
		variant := p3VariantExercised(c, observed, in)
		e.VariantVerified = e.VariantVerified && variant
		e.Requests = append(e.Requests, p3R03Request{Run: in.run, Gateway: in.gateway, Seq: in.seq, Turn: turn, Method: in.method, Path: in.recordPath, Status: in.status, Category: in.outcome, ConfirmationSHA256: in.confirmationSHA256, VariantVerified: variant})
		if in.run != e.Run.Index || in.gateway != e.Run.Gateway || in.seq <= 0 || in.method != "POST" || in.recordPath != in.path ||
			in.status != 503 || in.outcome != "NATIVE_CONFIRMATION_TIMEOUT" || in.nativeRouteScope != "conversation" || len(in.confirmationSHA256) != 64 {
			e.Complete = false
		}
	}
	if len(e.Requests) == 0 {
		e.Complete = false
		e.VariantVerified = false
	}
	if !e.Run.NaturalExit && !p3R03WindowProved(e.Run.Window, e.Requests) {
		e.Complete = false
	}
	e.Complete = e.Complete && e.VariantVerified
	return e
}
