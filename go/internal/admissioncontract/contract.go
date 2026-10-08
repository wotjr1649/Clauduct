// Package admissioncontract is the frozen provider-control admission contract (#320) and the
// routing contract it builds on, embedded byte for byte so a public build can reproduce what
// the gate and the probe decide from. Both read their case lists, restrictions and
// dispositions from here, and both put these files' SHA-256 in the admission cache key.
package admissioncontract

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
)

//go:embed admission-r3.json
var admission []byte

//go:embed routing-r4.json
var routing []byte

// ConfigurationID is the measured configuration (admission r2 configurations).
const ConfigurationID = "HOST_MANAGED_RESTRICTED"

// Scope is one admission scope's case sets.
type Scope struct {
	Required              []string `json:"requiredCases"`
	Optional              []string `json:"optionalCases"`
	BackgroundEntryChecks []string `json:"backgroundEntryChecks"`
	RequiresScope         []string `json:"requiresScope"`
}

// Disposition is one owner decision recorded in the contract.
type Disposition struct {
	Case        string `json:"case"`
	Label       string `json:"label"`
	Reason      string `json:"reason"`
	AppliesWhen string `json:"appliesWhen"`
	// AppliesWhenReason, when set, must also be among the case's reasons in the run.
	AppliesWhenReason string            `json:"appliesWhenReason"`
	ValidFor          map[string]string `json:"validFor"`
	// EquivalentGate, when set, must pass in the same run for the disposition to hold.
	EquivalentGate string `json:"equivalentGate"`
	Authority      string `json:"authority"`
}

// RunObserved is a disposition's validFor.nativeSHA256 that names no SHA: it holds for the
// native the run itself pinned, and the entry keyed by that SHA carries it (admission r3
// caseFixtureDisposition.runObserved).
const RunObserved = "RUN_OBSERVED"

type document struct {
	Revision       int              `json:"admissionContractRevision"`
	Scopes         map[string]Scope `json:"scopes"`
	Dispositions   []Disposition    `json:"dispositions"`
	Gaps           []string         `json:"gaps"`
	Configurations struct {
		Restricted struct {
			Plus map[string]string `json:"plus"`
		} `json:"HOST_MANAGED_RESTRICTED"`
	} `json:"configurations"`
}

var parsed = func() document {
	var d document
	if err := json.Unmarshal(admission, &d); err != nil {
		panic("admissioncontract: embedded admission contract: " + err.Error())
	}
	return d
}()

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// AdmissionSHA256 and RoutingSHA256 are the embedded contracts' digests.
func AdmissionSHA256() string { return digest(admission) }
func RoutingSHA256() string   { return digest(routing) }

// Revision is the admission contract revision.
func Revision() int { return parsed.Revision }

// Scopes returns each scope's case sets.
func Scopes() map[string]Scope { return parsed.Scopes }

// Restrictions are the configuration's enforced restrictions, sorted.
func Restrictions() []string {
	names := []string{}
	for name := range parsed.Configurations.Restricted.Plus {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Dispositions are the owner decisions the contract records.
func Dispositions() []Disposition { return parsed.Dispositions }

// Gaps are the contract's open gaps: what the frozen measurements and dispositions leave
// uncovered. Doctor lists them with the release blockers.
func Gaps() []string { return slices.Clone(parsed.Gaps) }

// ShortCases is a scope's short entry case set: required, optional and background-entry
// checks without RP01 (admission r2 caseVerdict.entries.short).
func ShortCases(scope string) []string {
	s := parsed.Scopes[scope]
	out := []string{}
	for _, list := range [][]string{s.Required, s.Optional, s.BackgroundEntryChecks} {
		for _, id := range list {
			if id != "RP01" {
				out = append(out, id)
			}
		}
	}
	return out
}

// NeedsRP01 reports whether the scope lists RP01 as required.
func NeedsRP01(scope string) bool { return slices.Contains(parsed.Scopes[scope].Required, "RP01") }
