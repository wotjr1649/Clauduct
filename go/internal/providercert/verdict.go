package providercert

import (
	"fmt"
	"slices"
	"sort"
)

// Revisions of the frozen routing contract and probe schema this build implements. Raising
// either invalidates every cached certification.
const (
	RoutingContractRevision = 1
	ProbeSchemaRevision     = 1
)

// Case outcomes.
const (
	Pass           = "PASS"
	FailRouting    = "FAIL_ROUTING"
	Inconclusive   = "INCONCLUSIVE"
	NotExercisable = "NOT_EXERCISABLE"
)

// Strategy outcomes.
const (
	Certified    = "CERTIFIED"
	Incompatible = "INCOMPATIBLE"
)

// Reasons.
const (
	ReasonStoredGatewayContact  = "STORED_GATEWAY_CONTACT"
	ReasonSettingsTargetContact = "SETTINGS_TARGET_CONTACT"
	ReasonCatchAllInference     = "CATCH_ALL_INFERENCE"
	ReasonCatchAllCanary        = "CATCH_ALL_CANARY"
	ReasonForeignMarker         = "FOREIGN_REPLY_MARKER"
	ReasonServedWithoutCanary   = "INTENDED_SERVED_WITHOUT_CANARY"
	ReasonSpawnFailed           = "SPAWN_FAILED"
	ReasonTimeout               = "TIMEOUT"
	ReasonResultMissing         = "RESULT_MISSING"
	ReasonBindFailed            = "LOOPBACK_BIND_FAILED"
	ReasonReceiverFault         = "RECEIVER_FAULT"
	ReasonNativeError           = "NATIVE_ERROR_BEFORE_INFERENCE"
	ReasonUnobservedReply       = "UNOBSERVED_REPLY"
	ReasonUndecodable           = "UNDECODABLE_EGRESS"
	ReasonTrafficNotCovered     = "PROVIDER_TRAFFIC_NOT_COVERED"
	ReasonHeldProcess           = "HELD_PROCESS"
	ReasonCleanupFailed         = "CLEANUP_FAILED"
	ReasonHarness               = "HARNESS_INTEGRITY"
	ReasonControlUnproven       = "CONTROL_PREREQUISITE_UNPROVEN"
	ReasonNativeChanged         = "NATIVE_CHANGED_DURING_CERTIFICATION"
	ReasonIntendedMissing       = "INTENDED_CANARY_MISSING"
	ReasonIntendedMarkerMissing = "INTENDED_MARKER_MISSING"
	ReasonMatrixMismatch        = "MATRIX_RESULTS_MISMATCH"
	ReasonUnattributedContact   = "UNATTRIBUTED_CONTACT"
)

// Wrong is what one wrong loopback destination saw.
type Wrong struct {
	Bytes, Conns int64
}

// Turn is one prompt of a case, as the runner observed it.
type Turn struct {
	// IntendedCanary counts requests at INTENDED, authenticated with the session token, whose
	// body carried this turn's canary.
	IntendedCanary int
	// IntendedInference counts inference-class requests at INTENDED in this turn's window.
	IntendedInference int
	// ControlInference is what the wrong provider received in the same turn of the paired
	// control. P8 compares against it.
	ControlInference int
	// IntendedServed is whether INTENDED answered a request of this turn.
	IntendedServed bool
	// Markers are the receiver IDs whose reply markers appear in native's output for this turn.
	Markers []string
	// UnmarkedReply is an assistant reply in the output that carries no probe marker.
	UnmarkedReply bool
}

// Harness are the run's own failures. Any of them makes the run unusable as evidence.
type Harness struct {
	SpawnFailed, TimedOut, ResultMissing, BindFailed, ReceiverFault   bool
	HeldProcess, CleanupFailed, Integrity, NativeErrorBeforeInference bool
}

// Control is the paired UNPROTECTED run.
type Control struct {
	// Fault is a control run that could not be judged: its own harness failed.
	Fault bool
	// Established is whether the synthetic state moved an unprotected native's request to the
	// wrong destination the case names.
	Established bool
}

// Observation is everything the verdict reads for one treatment case.
type Observation struct {
	CaseID string
	// Optional is a case that is not an anchor: its control may fail to move an unprotected
	// native, and then the case is NOT_EXERCISABLE rather than inconclusive.
	Optional bool
	// StoredGateway is WRONG_STORED_GATEWAY_A and _B; Settings is WRONG_SETTINGS and
	// WRONG_SOCKET.
	StoredGateway, Settings []Wrong
	// CanaryAttribution is set for a case where another process may legitimately reach the
	// stored gateway (R08b). Then only StoredGatewayCanary, the requests that carried this
	// case's canary, is a violation, and contact without it cannot be attributed.
	CanaryAttribution   bool
	StoredGatewayCanary int
	CatchAllInference   int
	CatchAllCanary      int
	CatchAllUndecodable int
	Turns               []Turn
	IdentityChanged     bool
	Harness             Harness
	Control             Control
}

// Result is one case's verdict.
type Result struct {
	CaseID  string   `json:"caseId"`
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons"`
	// IdentityChanged is carried so the strategy verdict can discard the whole run.
	IdentityChanged bool `json:"identityChanged"`
}

// IntendedID is the receiver ID INTENDED's markers carry.
const IntendedID = "INTENDED"

// Judge applies the contract's caseVerdict. A routing violation is decided first and is never
// softened by a missing control or a harness failure: bytes that reached a wrong destination
// are evidence on their own.
func Judge(o Observation) Result {
	r := Result{CaseID: o.CaseID, IdentityChanged: o.IdentityChanged}
	add := func(reason string) { r.Reasons = append(r.Reasons, reason) }

	storedContact := false
	for _, w := range o.StoredGateway {
		storedContact = storedContact || w.Bytes != 0 || w.Conns != 0
	}
	if o.CanaryAttribution {
		if o.StoredGatewayCanary != 0 {
			add(ReasonStoredGatewayContact)
		}
	} else if storedContact {
		add(ReasonStoredGatewayContact)
	}
	for _, w := range o.Settings {
		if w.Bytes != 0 || w.Conns != 0 {
			add(ReasonSettingsTargetContact)
			break
		}
	}
	if o.CatchAllInference != 0 {
		add(ReasonCatchAllInference)
	}
	if o.CatchAllCanary != 0 {
		add(ReasonCatchAllCanary)
	}
	for _, t := range o.Turns {
		if slices.ContainsFunc(t.Markers, func(id string) bool { return id != IntendedID }) {
			add(ReasonForeignMarker)
			break
		}
	}
	for _, t := range o.Turns {
		if t.IntendedServed && t.IntendedCanary == 0 {
			add(ReasonServedWithoutCanary)
			break
		}
	}
	if len(r.Reasons) > 0 {
		r.Verdict = FailRouting
		return r
	}

	h := o.Harness
	for _, f := range []struct {
		on     bool
		reason string
	}{{o.IdentityChanged, ReasonNativeChanged}, {h.SpawnFailed, ReasonSpawnFailed}, {h.TimedOut, ReasonTimeout},
		{h.ResultMissing, ReasonResultMissing}, {h.BindFailed, ReasonBindFailed}, {h.ReceiverFault, ReasonReceiverFault},
		{h.HeldProcess, ReasonHeldProcess}, {h.CleanupFailed, ReasonCleanupFailed}, {h.Integrity, ReasonHarness}} {
		if f.on {
			add(f.reason)
		}
	}
	if o.Control.Fault {
		add(ReasonControlUnproven)
	}
	if o.CanaryAttribution && storedContact {
		add(ReasonUnattributedContact)
	}
	if len(r.Reasons) > 0 {
		r.Verdict = Inconclusive
		return r
	}
	// What the treatment itself failed to show, apart from the comparison with its control.
	if h.NativeErrorBeforeInference {
		add(ReasonNativeError)
	}
	if o.CatchAllUndecodable != 0 {
		add(ReasonUndecodable)
	}
	if len(o.Turns) == 0 {
		add(ReasonResultMissing)
	}
	for _, t := range o.Turns {
		if t.IntendedCanary == 0 {
			add(ReasonIntendedMissing)
			break
		}
	}
	for _, t := range o.Turns {
		if !slices.Contains(t.Markers, IntendedID) {
			add(ReasonIntendedMarkerMissing)
			break
		}
	}
	for _, t := range o.Turns {
		if t.UnmarkedReply && !t.IntendedServed {
			add(ReasonUnobservedReply)
			break
		}
	}
	if !o.Control.Established {
		// NOT_EXERCISABLE only for a treatment that is clean on its own (P1-P7); P8 needs the
		// control and is not read. An anchor without a control proves nothing.
		if o.Optional && len(r.Reasons) == 0 {
			r.Verdict = NotExercisable
			return r
		}
		if !o.Optional {
			r.Reasons = append([]string{ReasonControlUnproven}, r.Reasons...)
		}
		r.Verdict = Inconclusive
		return r
	}
	for _, t := range o.Turns {
		if t.IntendedInference < t.ControlInference {
			add(ReasonTrafficNotCovered)
			break
		}
	}
	if len(r.Reasons) > 0 {
		r.Verdict = Inconclusive
		return r
	}
	r.Verdict = Pass
	return r
}

// Strategy is one strategy's certification verdict.
type Strategy struct {
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons"`
	Cases   []Result `json:"cases"`
	// NotExercisable lists the optional cases whose control did not move an unprotected native.
	NotExercisable []string `json:"notExercisable"`
}

// Decide applies the contract's strategyVerdict to the results of one certification run.
// matrix is the exact list of case IDs the run had to produce and optional the subset that may
// be NOT_EXERCISABLE (every case that is not an anchor).
func Decide(matrix, optional []string, results []Result) Strategy {
	s := Strategy{Cases: results}
	for _, r := range results {
		if r.IdentityChanged {
			s.Verdict, s.Reasons = Inconclusive, []string{ReasonNativeChanged}
			return s
		}
	}
	for _, r := range results {
		if r.Verdict == FailRouting {
			s.Verdict = Incompatible
			s.Reasons = append(s.Reasons, fmt.Sprintf("%s:%s", r.CaseID, FailRouting))
		}
	}
	if s.Verdict == Incompatible {
		return s
	}
	if !sameSet(matrix, results) {
		s.Verdict, s.Reasons = Inconclusive, []string{ReasonMatrixMismatch}
		return s
	}
	for _, r := range results {
		switch r.Verdict {
		case Pass:
		case NotExercisable:
			if !slices.Contains(optional, r.CaseID) {
				s.Verdict = Inconclusive
				s.Reasons = append(s.Reasons, r.CaseID+":"+ReasonControlUnproven)
				continue
			}
			s.NotExercisable = append(s.NotExercisable, r.CaseID)
		default:
			s.Verdict = Inconclusive
			s.Reasons = append(s.Reasons, r.CaseID+":"+r.Verdict)
		}
	}
	if s.Verdict == Inconclusive {
		return s
	}
	s.Verdict = Certified
	return s
}

func sameSet(matrix []string, results []Result) bool {
	if len(matrix) != len(results) {
		return false
	}
	want := slices.Clone(matrix)
	got := make([]string, 0, len(results))
	for _, r := range results {
		got = append(got, r.CaseID)
	}
	sort.Strings(want)
	sort.Strings(got)
	return slices.Equal(want, got) && len(slices.Compact(slices.Clone(got))) == len(got)
}
