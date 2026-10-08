package providercert

import (
	"fmt"
	"slices"
	"sort"
)

// Revisions of the frozen routing contract and probe schema this build implements. Raising
// either invalidates every cached certification. Revision 2 split P5 into routing liveness
// (P5-R) and native preservation (P5-F). Revision 3 gives each case its own expected refusals,
// adds the hook-diagnostic expectation, the SAFETY purpose for same-process state changes and
// the observed state application. Earlier results stay judged under their own revision.
const (
	RoutingContractRevision = 4
	ProbeSchemaRevision     = 4
)

// Case outcomes.
const (
	Pass           = "PASS"
	FailRouting    = "FAIL_ROUTING"
	FailFunction   = "FAIL_FUNCTION"
	FailSafety     = "FAIL_SAFETY"
	Inconclusive   = "INCONCLUSIVE"
	NotExercisable = "NOT_EXERCISABLE"
	// SafetyPass is a SAFETY-purpose case that met its requirements. It shows the state change
	// was applied and the session stayed correct, not that the strategy suppressed a threat.
	SafetyPass = "SAFETY_PASS"
	// SafeRefusalPass is a SAFE_REFUSAL-purpose case: the product refused as its contract says,
	// with no backend generation and no side effect. It is not the refused feature working.
	SafeRefusalPass = "SAFE_REFUSAL_PASS"
)

// Case purposes.
const (
	// PurposeSuppression compares a treatment with a control that reproduced the threat.
	PurposeSuppression = ""
	// PurposeSafety checks that a state change is applied and the session stays correct; it
	// claims nothing about suppression, so its control is documentation only.
	PurposeSafety = "SAFETY"
	// PurposeSafeRefusal checks an intended refusal (a contract the product keeps), not a
	// working feature and not suppression.
	PurposeSafeRefusal = "SAFE_REFUSAL"
)

// Strategy outcomes.
const (
	Certified    = "CERTIFIED"
	Incompatible = "INCOMPATIBLE"
)

// What a turn's native preservation check (P5-F) expects.
const (
	ExpectSuccess = "SUCCESS"
	// ExpectSuccessHookDiagnostic is a turn that completes although a command hook failed, and
	// native recorded that hook's failure.
	ExpectSuccessHookDiagnostic = "SUCCESS_WITH_HOOK_DIAGNOSTIC"
	ExpectFailClosed            = "EXPECTED_FAIL_CLOSED"
	ExpectStimulus              = "STIMULUS"
	// ExpectSafeRefusal is a turn the product refuses before any model request (a session that
	// must be resumed explicitly): no request is expected at INTENDED, so P4, P5-R and P8 are
	// not read for it; the refusal, zero backend generation and no side effect are.
	ExpectSafeRefusal = "EXPECTED_SAFE_REFUSAL"
)

// ExpectedRefusals are the refusals a fail-closed turn is held to when its case names none
// (revision 2's degraded-hook expectation).
var ExpectedRefusals = []string{"NATIVE_REQUEST_ORIGIN_UNVERIFIED", "NATIVE_CONFIRMATION_UNVERIFIED"}

// Reasons.
const (
	ReasonStoredGatewayContact  = "STORED_GATEWAY_CONTACT"
	ReasonSettingsTargetContact = "SETTINGS_TARGET_CONTACT"
	ReasonCatchAllInference     = "CATCH_ALL_INFERENCE"
	ReasonCatchAllCanary        = "CATCH_ALL_CANARY"
	ReasonForeignMarker         = "FOREIGN_REPLY_MARKER"
	ReasonForeignCanary         = "FOREIGN_RECEIVER_CANARY"
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
	ReasonAuxiliaryOnly         = "AUXILIARY_ONLY"
	ReasonMatrixMismatch        = "MATRIX_RESULTS_MISMATCH"
	ReasonUnattributedContact   = "UNATTRIBUTED_CONTACT"
	ReasonPreservationFailed    = "NATIVE_PRESERVATION_FAILED"
	ReasonFailClosedNotObserved = "FAIL_CLOSED_NOT_OBSERVED"
	ReasonUnknownExpectation    = "UNKNOWN_EXPECTATION"
	ReasonHookDiagnosticMissing = "HOOK_DIAGNOSTIC_MISSING"
	ReasonStateNotApplied       = "STATE_NOT_APPLIED"
	ReasonApplicationUnobserved = "STATE_APPLICATION_UNOBSERVED"
	ReasonForeignSessionChanged = "FOREIGN_SESSION_STATE_CHANGED"
	ReasonVariantNotExercised   = "VARIANT_NOT_EXERCISED"
	ReasonSafeRefusalMissing    = "SAFE_REFUSAL_NOT_OBSERVED"
	ReasonRestoreMissing        = "RESTORE_NOT_OBSERVED"
)

// Wrong is what one wrong loopback destination saw.
type Wrong struct {
	Bytes, Conns int64
}

// Turn is one prompt of a case, as the runner observed it.
type Turn struct {
	// Expect is what P5-F requires of this turn: ExpectSuccess, ExpectSuccessHookDiagnostic,
	// ExpectFailClosed or ExpectStimulus.
	Expect string
	// FailClosedAs are the refusal categories an ExpectFailClosed turn must end with; empty
	// means ExpectedRefusals.
	FailClosedAs []string
	// HookDiagnostic is native's record of the failed command hook in this turn.
	HookDiagnostic bool
	// For an ExpectSafeRefusal turn: the product's refusal text with its exact recovery
	// command was observed, how many backend generations carried this turn's canary, and
	// whether stored state (profile snapshot, earlier conversation) changed.
	SafeRefusal  bool
	BackendCalls int
	SideEffect   bool
	// RequireRestore is a turn that must continue an earlier conversation; Restored is its model
	// request carrying the earlier turn's canary and the same model.
	RequireRestore, Restored bool
	// IntendedCanary counts requests at INTENDED's ingress, authenticated with the session token,
	// whose body carried this turn's canary (P4).
	IntendedCanary int
	// IntendedMainCanary is the part of IntendedCanary whose request class was not auxiliary:
	// the turn itself rather than a title or classifier that quoted it (P5-R).
	IntendedMainCanary int
	// Refusals are the gateway's categories for this turn's canary-bearing ingress requests,
	// matched to them by the gateway's record number; "" for one that was not refused.
	Refusals []string
	// IntendedInference counts inference-class requests at INTENDED in this turn's window.
	IntendedInference int
	// ControlInference is what the wrong provider received in the same turn of the paired
	// control. P8 compares against it.
	ControlInference int
	// IntendedServed is whether INTENDED's backend answered a conversation-bearing request of
	// this turn.
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
	// Purpose is PurposeSuppression or PurposeSafety.
	Purpose string
	// StateNotApplied is a stateful case whose synthetic state did not read back as intended
	// after a step; ApplicationUnobserved one whose native was not seen to apply it before the
	// next turn although the case requires that observation.
	StateNotApplied, ApplicationUnobserved bool
	// ForeignSessionChanged is another live session's gateway state changed during the case.
	ForeignSessionChanged bool
	// VariantUnexercised is a case whose own condition did not happen (a late hook event that
	// never arrived, a foreign link never used), so its result shows nothing about it.
	VariantUnexercised bool
	// ForeignCanary counts requests carrying this case's canary at any probe receiver other
	// than INTENDED, including one no byte rule covers (STANDALONE_STORED_GATEWAY_C).
	ForeignCanary       int
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
// are evidence on their own. Native preservation (P5-F) is judged last, and only for a case
// whose routing evidence is complete.
//
// A case whose own condition did not happen (VariantUnexercised) is VARIANT_NOT_EXERCISED only
// when that is the sole unmet condition: every other rule is judged as if the variant had
// happened, and a case that fails any of them keeps its real verdict and reasons, so the
// disposition that reads the variant reason cannot apply to it (admission r3 dispositions).
func Judge(o Observation) Result {
	if !o.VariantUnexercised {
		return judge(o)
	}
	rest := o
	rest.VariantUnexercised = false
	r := judge(rest)
	switch r.Verdict {
	case Pass, SafetyPass, SafeRefusalPass:
		r.Verdict, r.Reasons = Inconclusive, []string{ReasonVariantNotExercised}
	case NotExercisable:
		// An optional case that is inconclusive for its variant is not exercisable-and-clean.
		r.Verdict, r.Reasons = Inconclusive, []string{ReasonControlUnproven}
	case FailFunction:
		// The routing contract keeps an unexercised variant INCONCLUSIVE (r4
		// caseVerdict.variantExercised): a preservation failure seen without the variant is
		// reported with its reasons, never as a product defect, and without the variant reason
		// no disposition can read it.
		r.Verdict = Inconclusive
	}
	return r
}

func judge(o Observation) Result {
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
	if o.ForeignCanary != 0 {
		add(ReasonForeignCanary)
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
	if o.ForeignSessionChanged {
		r.Verdict, r.Reasons = FailSafety, []string{ReasonForeignSessionChanged}
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
	if o.Control.Fault && o.Purpose != PurposeSafety && o.Purpose != PurposeSafeRefusal {
		add(ReasonControlUnproven)
	}
	if o.StateNotApplied {
		add(ReasonStateNotApplied)
	}
	if o.ApplicationUnobserved {
		add(ReasonApplicationUnobserved)
	}
	if o.CanaryAttribution && storedContact {
		add(ReasonUnattributedContact)
	}
	if len(r.Reasons) > 0 {
		r.Verdict = Inconclusive
		return r
	}

	// What the treatment itself failed to show for routing (P3 undecodable, P4, P5-R).
	if o.CatchAllUndecodable != 0 {
		add(ReasonUndecodable)
	}
	if len(o.Turns) == 0 {
		add(ReasonResultMissing)
	}
	for _, t := range o.Turns {
		switch {
		case t.Expect == ExpectSafeRefusal:
			continue
		case t.IntendedCanary == 0:
			add(ReasonIntendedMissing)
		case t.IntendedMainCanary == 0:
			add(ReasonAuxiliaryOnly)
		default:
			continue
		}
		break
	}
	for _, t := range o.Turns {
		if t.Expect != ExpectSafeRefusal && t.UnmarkedReply && t.IntendedCanary == 0 && !t.IntendedServed {
			add(ReasonUnobservedReply)
			break
		}
	}
	for _, t := range o.Turns {
		if !slices.Contains([]string{ExpectSuccess, ExpectSuccessHookDiagnostic, ExpectFailClosed, ExpectStimulus, ExpectSafeRefusal}, t.Expect) {
			add(ReasonUnknownExpectation)
			break
		}
	}
	if len(r.Reasons) > 0 && h.NativeErrorBeforeInference {
		r.Reasons = append([]string{ReasonNativeError}, r.Reasons...)
	}
	function := preservation(o.Turns)
	if o.Purpose == PurposeSafeRefusal && !slices.ContainsFunc(o.Turns, func(t Turn) bool { return t.Expect == ExpectSafeRefusal }) {
		function = append(function, ReasonSafeRefusalMissing)
	}
	if o.Purpose == PurposeSafety || o.Purpose == PurposeSafeRefusal {
		// The control documents whether the threat appeared; a safety case is not judged on it,
		// and P8 compares against it, so neither is read.
		if len(r.Reasons) > 0 {
			r.Reasons = append(r.Reasons, function...)
			r.Verdict = Inconclusive
			return r
		}
		if len(function) > 0 {
			r.Verdict, r.Reasons = FailFunction, function
			return r
		}
		r.Verdict = SafetyPass
		if o.Purpose == PurposeSafeRefusal {
			r.Verdict = SafeRefusalPass
		}
		return r
	}
	if !o.Control.Established {
		// NOT_EXERCISABLE only for a treatment that is clean on its own, routing and native
		// preservation both; P8 needs the control and is not read. An anchor without a control
		// proves nothing.
		if o.Optional && len(r.Reasons) == 0 && len(function) == 0 {
			r.Verdict = NotExercisable
			return r
		}
		if !o.Optional {
			r.Reasons = append([]string{ReasonControlUnproven}, r.Reasons...)
		}
		r.Reasons = append(r.Reasons, function...)
		r.Verdict = Inconclusive
		return r
	}
	for _, t := range o.Turns {
		if t.Expect != ExpectSafeRefusal && t.IntendedInference < t.ControlInference {
			add(ReasonTrafficNotCovered)
			break
		}
	}
	if len(r.Reasons) > 0 {
		r.Verdict = Inconclusive
		return r
	}
	if len(function) > 0 {
		r.Verdict, r.Reasons = FailFunction, function
		return r
	}
	r.Verdict = Pass
	return r
}

// preservation applies P5-F to each turn and returns the reasons it fails, if any.
func preservation(turns []Turn) []string {
	var reasons []string
	note := func(reason string) {
		if !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	for _, t := range turns {
		switch t.Expect {
		case ExpectSuccess, ExpectSuccessHookDiagnostic:
			if !slices.Contains(t.Markers, IntendedID) {
				note(ReasonPreservationFailed)
			}
			if t.Expect == ExpectSuccessHookDiagnostic && !t.HookDiagnostic {
				note(ReasonHookDiagnosticMissing)
			}
			if t.RequireRestore && !t.Restored {
				note(ReasonRestoreMissing)
			}
		case ExpectSafeRefusal:
			if !t.SafeRefusal || t.BackendCalls != 0 || t.SideEffect || t.IntendedServed || len(t.Markers) != 0 {
				note(ReasonSafeRefusalMissing)
			}
		case ExpectFailClosed:
			allowed := t.FailClosedAs
			if len(allowed) == 0 {
				allowed = ExpectedRefusals
			}
			refused := len(t.Refusals) > 0
			for _, category := range t.Refusals {
				refused = refused && slices.Contains(allowed, category)
			}
			if !refused || t.IntendedServed || slices.Contains(t.Markers, IntendedID) {
				note(ReasonFailClosedNotObserved)
			}
		}
	}
	return reasons
}

// Strategy is one strategy's certification verdict.
type Strategy struct {
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons"`
	Cases   []Result `json:"cases"`
	// NotExercisable lists the optional cases whose control did not move an unprotected native.
	NotExercisable []string `json:"notExercisable"`
	// PreservationFailures lists the cases that routed correctly but failed P5-F, which is a
	// product or surface defect to report.
	PreservationFailures []string `json:"preservationFailures,omitempty"`
	// SafetyCases lists the SAFETY-purpose cases that passed: evidence about state changes,
	// not about suppression.
	SafetyCases []string `json:"safetyCases,omitempty"`
	// SafetyFailures lists the cases that changed another live session's state.
	SafetyFailures []string `json:"safetyFailures,omitempty"`
	// SafeRefusals lists the SAFE_REFUSAL cases that passed: refusals kept, not features working.
	SafeRefusals []string `json:"safeRefusals,omitempty"`
}

// Decide applies the contract's strategyVerdict to the results of one certification run.
// matrix is the exact list of case IDs the run had to produce, optional the subset that may
// be NOT_EXERCISABLE (every case that is not an anchor) and safety the SAFETY-purpose subset,
// whose only passing verdict is SAFETY_PASS.
func Decide(matrix, optional, safety []string, results []Result, refusals ...string) Strategy {
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
		isSafety := slices.Contains(safety, r.CaseID)
		switch r.Verdict {
		case Pass:
			if isSafety || slices.Contains(refusals, r.CaseID) {
				s.Verdict = Inconclusive
				s.Reasons = append(s.Reasons, r.CaseID+":"+r.Verdict)
			}
		case SafeRefusalPass:
			if !slices.Contains(refusals, r.CaseID) {
				s.Verdict = Inconclusive
				s.Reasons = append(s.Reasons, r.CaseID+":"+r.Verdict)
				continue
			}
			s.SafeRefusals = append(s.SafeRefusals, r.CaseID)
		case SafetyPass:
			if !isSafety {
				s.Verdict = Inconclusive
				s.Reasons = append(s.Reasons, r.CaseID+":"+r.Verdict)
				continue
			}
			s.SafetyCases = append(s.SafetyCases, r.CaseID)
		case FailSafety:
			s.SafetyFailures = append(s.SafetyFailures, r.CaseID)
		case NotExercisable:
			if !slices.Contains(optional, r.CaseID) {
				s.Verdict = Inconclusive
				s.Reasons = append(s.Reasons, r.CaseID+":"+ReasonControlUnproven)
				continue
			}
			s.NotExercisable = append(s.NotExercisable, r.CaseID)
		case FailFunction:
			s.PreservationFailures = append(s.PreservationFailures, r.CaseID)
		default:
			s.Verdict = Inconclusive
			s.Reasons = append(s.Reasons, r.CaseID+":"+r.Verdict)
		}
	}
	if s.Verdict == Inconclusive {
		return s
	}
	if len(s.SafetyFailures) > 0 {
		s.Verdict, s.Reasons = Inconclusive, []string{ReasonForeignSessionChanged}
		return s
	}
	if len(s.PreservationFailures) > 0 {
		s.Verdict, s.Reasons = Inconclusive, []string{ReasonPreservationFailed}
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
