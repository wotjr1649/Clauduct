package providerprobe

import (
	"slices"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
)

// Case verdicts the admission judgment reads beyond r4's (admission r2 backgroundEntryChecks,
// caseFixtureDisposition).
const (
	VerdictBackgroundEntryReached = "BACKGROUND_ENTRY_REACHED"
	VerdictPathAbsent             = "PATH_ABSENT"
	VerdictFixtureNotApplied      = "FIXTURE_NOT_APPLIED"
	VerdictVariantNotExercised    = "VARIANT_NOT_EXERCISED"
	VerdictCaseNotImplemented     = "CASE_NOT_IMPLEMENTED"
)

// CaseResult is one case's verdict in a run, with what the contract says about the case.
type CaseResult struct {
	Verdict string
	Reasons []string
	// Purpose is the case's r4 purpose: SAFETY, SAFE_REFUSAL or "" (suppression).
	Purpose string
	// CLIBackgroundEntry marks a CLI background-entry check, the only kind PATH_ABSENT may come from.
	CLIBackgroundEntry bool
}

// AdmissionInput is everything one scope's judgment reads.
type AdmissionInput struct {
	Scope           string
	Results         map[string]CaseResult
	IdentityChanged bool
	// NativeSHA256 and Platform select which dispositions are valid.
	NativeSHA256, Platform string
	// GatesPassed are the equivalent gates this run showed working, by disposition case.
	GatesPassed map[string]bool
	// Required are the states of the scope's requiresScope entries.
	Required map[string]string
}

// AdmissionOutcome is a scope's state and each case's label.
type AdmissionOutcome struct {
	State   string
	Reasons []string
	Labels  map[string]string
}

// Admit applies the admission contract's caseVerdict.order to one scope's short run.
func Admit(in AdmissionInput) AdmissionOutcome {
	out := AdmissionOutcome{Labels: map[string]string{}}
	inconclusive := func(reason string) AdmissionOutcome {
		out.State, out.Reasons = app.AdmissionInconclusive, append(out.Reasons, reason)
		return out
	}
	// 1
	if in.IdentityChanged {
		return inconclusive("NATIVE_CHANGED_DURING_CERTIFICATION")
	}
	// 2
	for id, r := range in.Results {
		if r.Verdict == "FAIL_ROUTING" || r.Verdict == VerdictBackgroundEntryReached {
			out.State, out.Reasons = app.AdmissionNotAdmitted, []string{id + ":" + r.Verdict}
			return out
		}
	}
	// 3
	want := admissioncontract.ShortCases(in.Scope)
	if len(want) == 0 || len(in.Results) != len(want) {
		return inconclusive("MATRIX_RESULTS_MISMATCH")
	}
	for _, id := range want {
		if _, ok := in.Results[id]; !ok {
			return inconclusive("MATRIX_RESULTS_MISMATCH")
		}
	}
	optional := admissioncontract.Scopes()[in.Scope].Optional
	// Dispositions apply only when the run's own verdict (and, when the disposition names one,
	// its reason) is the one they were decided for, on this platform, and for this native: a
	// RUN_OBSERVED disposition holds for whatever native this run pinned, because the entry it
	// lands in is keyed by that native (admission r3 caseFixtureDisposition.runObserved).
	disposed := map[string]admissioncontract.Disposition{}
	for _, d := range admissioncontract.Dispositions() {
		r, ok := in.Results[d.Case]
		if !ok || r.Verdict != d.AppliesWhen || d.ValidFor["platform"] != in.Platform {
			continue
		}
		if d.AppliesWhenReason != "" && !slices.Contains(r.Reasons, d.AppliesWhenReason) {
			continue
		}
		if native := d.ValidFor["nativeSHA256"]; native != admissioncontract.RunObserved && native != in.NativeSHA256 {
			continue
		}
		disposed[d.Case] = d
	}
	// 4, 5, 6 and 6b all leave the scope INCONCLUSIVE; each names its case.
	var reasons []string
	note := func(reason string) { reasons = append(reasons, reason) }
	for _, id := range want {
		r := in.Results[id]
		if d, ok := disposed[id]; ok {
			out.Labels[id] = d.Label
			continue
		}
		switch {
		case r.Verdict == "PASS" && r.Purpose == "SAFETY":
			note(id + ":SAFETY_CASE_PASS")
		case r.Verdict == "PASS" && r.Purpose == "SAFE_REFUSAL":
			note(id + ":SAFE_REFUSAL_CASE_PASS")
		case r.Verdict == "PASS":
			out.Labels[id] = "PASS"
		case r.Verdict == "SAFETY_PASS" && r.Purpose == "SAFETY":
			out.Labels[id] = "SAFETY_PASS"
		case r.Verdict == "SAFETY_PASS":
			note(id + ":SAFETY_PASS_OUTSIDE_SAFETY")
		case r.Verdict == "SAFE_REFUSAL_PASS" && r.Purpose == "SAFE_REFUSAL":
			out.Labels[id] = "SAFE_REFUSAL"
		case r.Verdict == "SAFE_REFUSAL_PASS":
			note(id + ":SAFE_REFUSAL_OUTSIDE_SAFE_REFUSAL")
		case r.Verdict == "NOT_EXERCISABLE" && slices.Contains(optional, id):
			out.Labels[id] = "NOT_EXERCISABLE"
		case r.Verdict == VerdictPathAbsent && r.CLIBackgroundEntry:
			out.Labels[id] = VerdictPathAbsent
		case r.Verdict == "FAIL_SAFETY":
			note(id + ":FOREIGN_SESSION_STATE_CHANGED")
		case r.Verdict == "FAIL_FUNCTION":
			note(id + ":NATIVE_PRESERVATION_FAILED")
		default:
			// INCONCLUSIVE, a required NOT_EXERCISABLE, an undisposed FIXTURE_NOT_APPLIED or
			// VARIANT_NOT_EXERCISED, a case not implemented, PATH_ABSENT from a non-CLI case.
			note(id + ":" + r.Verdict)
		}
	}
	if len(reasons) > 0 {
		out.State, out.Reasons = app.AdmissionInconclusive, reasons
		return out
	}
	// 7
	for id, label := range out.Labels {
		if label == "BLOCKING" {
			out.Reasons = append(out.Reasons, id+":BLOCKING")
		}
		// A disposition with an equivalent gate holds only when that gate passed in this run.
		if d, ok := disposed[id]; ok && d.EquivalentGate != "" && !in.GatesPassed[id] {
			out.Reasons = append(out.Reasons, id+":EQUIVALENT_GATE_NOT_SHOWN")
		}
	}
	slices.Sort(out.Reasons)
	if len(out.Reasons) > 0 {
		out.State = app.AdmissionInconclusive
		return out
	}
	for _, scope := range admissioncontract.Scopes()[in.Scope].RequiresScope {
		switch in.Required[scope] {
		case app.AdmissionAdmitted:
		case app.AdmissionNotAdmitted:
			out.State, out.Reasons = app.AdmissionNotAdmitted, []string{"REQUIRED_SCOPE_NOT_ADMITTED:" + scope}
			return out
		default:
			return inconclusive("REQUIRED_SCOPE_NOT_ADMITTED:" + scope)
		}
	}
	out.State = app.AdmissionAdmitted
	return out
}

// RP01Satisfied is admission r2's rule for a scope's separate RP01 entry.
func RP01Satisfied(verdict, label string) string {
	switch {
	case verdict == "PASS" && label == "SOURCE_INACTIVE":
		return app.AdmissionAdmitted
	case verdict == "FAIL_ROUTING":
		return app.AdmissionNotAdmitted
	}
	return app.AdmissionInconclusive
}
