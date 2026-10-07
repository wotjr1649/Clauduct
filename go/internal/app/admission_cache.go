package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
)

// Admission states (admission contract r1 states).
const (
	AdmissionAdmitted     = "ADMITTED"
	AdmissionNotAdmitted  = "NOT_ADMITTED"
	AdmissionInconclusive = "INCONCLUSIVE"
)

// AdmissionKey is the cache key of one scope's admission (admission contract r1 cache.key).
// Every field must match exactly; any difference is a MISS with its stale reason.
type AdmissionKey struct {
	NativeSHA256             string `json:"nativeSHA256"`
	ConfigurationID          string `json:"configurationId"`
	RoutingContractSHA256    string `json:"routingContractSHA256"`
	AdmissionContractSHA256  string `json:"admissionContractSHA256"`
	ProbeSchemaRevision      int    `json:"probeSchemaRevision"`
	ClauductBinarySHA256     string `json:"clauductBinarySHA256"`
	ManagedSourceFingerprint string `json:"managedSourceFingerprint"`
	Scope                    string `json:"scope"`
	RestrictionsDigest       string `json:"restrictionsDigest"`
	// ProbeExecutableSHA256 is the clauduct-probe.exe that wrote the entry (admission r2).
	ProbeExecutableSHA256 string `json:"probeExecutableSHA256"`
}

// AdmissionCase is one case's result in an entry.
type AdmissionCase struct {
	ID      string   `json:"id"`
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons,omitempty"`
	// Label is the admission r2 label the case was judged under (PASS, SAFETY_PASS, ...).
	Label string `json:"label,omitempty"`
}

// AdmissionEntry is one scope's cached admission.
type AdmissionEntry struct {
	Schema         string          `json:"schema"`
	Key            AdmissionKey    `json:"key"`
	State          string          `json:"state"`
	Reasons        []string        `json:"reasons,omitempty"`
	Cases          []AdmissionCase `json:"cases"`
	NotExercisable []string        `json:"notExercisable,omitempty"`
	Gaps           []string        `json:"gaps,omitempty"`
	EvidenceDigest string          `json:"evidenceDigest"`
	CreatedAt      time.Time       `json:"createdAt"`
	Probe          string          `json:"probeImplementation"`
}

const AdmissionSchema = "clauduct.provider-admission/1"

// RestrictionsDigest is the SHA-256 of the restriction names, sorted and joined by newlines.
func RestrictionsDigest(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

// RP01Scope names a scope's separate RP01 entry (admission r2 caseVerdict.entries).
func RP01Scope(scope string) string { return scope + "#RP01" }

// admissionDir is where entries live. A reparse point anywhere on it is not used.
func admissionDir(home string) string { return filepath.Join(home, ".clauduct", "provider-admission") }

// AdmissionMiss is a lookup that found no usable entry; its text is the stale reason.
type AdmissionMiss struct{ reason string }

func (e AdmissionMiss) Error() string { return "ADMISSION_MISS: " + e.reason }

// LookupAdmission returns the entry for key's scope when its key matches exactly. Anything
// else is a MISS: no file, an unreadable or unknown file, a reparse point, or a key that
// differs (the first differing field names the stale reason).
func LookupAdmission(home string, key AdmissionKey) (AdmissionEntry, error) {
	path := filepath.Join(admissionDir(home), key.Scope+".json")
	if reparsePath(path) {
		return AdmissionEntry{}, AdmissionMiss{"REPARSE_POINT"}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return AdmissionEntry{}, AdmissionMiss{"ABSENT"}
	}
	var entry AdmissionEntry
	if json.Unmarshal(raw, &entry) != nil || entry.Schema != AdmissionSchema {
		return AdmissionEntry{}, AdmissionMiss{"UNREADABLE"}
	}
	switch got := entry.Key; {
	case got.NativeSHA256 != key.NativeSHA256:
		return entry, AdmissionMiss{"NATIVE_CHANGED"}
	case got.ConfigurationID != key.ConfigurationID:
		return entry, AdmissionMiss{"CONFIGURATION_CHANGED"}
	case got.RoutingContractSHA256 != key.RoutingContractSHA256:
		return entry, AdmissionMiss{"ROUTING_CONTRACT_CHANGED"}
	case got.AdmissionContractSHA256 != key.AdmissionContractSHA256:
		return entry, AdmissionMiss{"ADMISSION_CONTRACT_CHANGED"}
	case got.ProbeSchemaRevision != key.ProbeSchemaRevision:
		return entry, AdmissionMiss{"PROBE_SCHEMA_CHANGED"}
	case got.ClauductBinarySHA256 != key.ClauductBinarySHA256:
		return entry, AdmissionMiss{"CLAUDUCT_BINARY_CHANGED"}
	case got.ManagedSourceFingerprint != key.ManagedSourceFingerprint:
		return entry, AdmissionMiss{"MANAGED_SOURCES_CHANGED"}
	case got.RestrictionsDigest != key.RestrictionsDigest:
		return entry, AdmissionMiss{"RESTRICTIONS_CHANGED"}
	case got.ProbeExecutableSHA256 != key.ProbeExecutableSHA256:
		return entry, AdmissionMiss{"PROBE_EXECUTABLE_CHANGED"}
	case got.Scope != key.Scope:
		return entry, AdmissionMiss{"UNREADABLE"}
	}
	return entry, nil
}

// WriteAdmission stores entry for its scope: a temporary file in the same directory, then a
// rename, so a reader sees the old entry or the new one and never a part.
func WriteAdmission(home string, entry AdmissionEntry) error {
	dir := admissionDir(home)
	if reparsePath(dir) {
		return errors.New("CACHE_WRITE_FAILED: the admission directory is a reparse point")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entry.Schema = AdmissionSchema
	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, entry.Key.Scope+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, entry.Key.Scope+".json")); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// reparsePath reports whether path or any existing parent is a reparse point (a junction or
// symbolic link): such an entry could be redirected and is not read or written.
func reparsePath(path string) bool {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if info, err := os.Lstat(p); err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
		if parent := filepath.Dir(p); parent == p {
			return false
		}
	}
}

// AdmissionScopes are the scopes in admission contract r1's order.
var AdmissionScopes = []string{ScopePrintSingle, ScopeResumeExplicit, ScopeContinuePrint, ScopeMultiTurn, ScopeTUI}

// AdmissionStatus is what doctor shows for one scope: the stored state and, under the key
// computed for what is installed now, why the gate would not use it. It never runs a probe.
type AdmissionStatus struct {
	Scope          string
	State          string // ADMITTED, NOT_ADMITTED, INCONCLUSIVE, or NONE when nothing is stored
	Reasons        []string
	CreatedAt      time.Time
	NotExercisable []string
	Gaps           []string
	// Labels are the cases judged under a label other than PASS, as "ID:LABEL".
	Labels []string
	// Stale is the stale reason under the current key ("" when the gate would use the entry).
	Stale string
}

// AdmissionStatuses reads each scope's stored entry (and RP01 entry) under home and checks it
// against key, the current key builder; a nil key marks every stored entry KEY_UNAVAILABLE.
func AdmissionStatuses(home string, key func(string) AdmissionKey) []AdmissionStatus {
	out := []AdmissionStatus{}
	scopes := []string{}
	for _, scope := range AdmissionScopes {
		scopes = append(scopes, scope)
		if admissioncontract.NeedsRP01(scope) {
			scopes = append(scopes, RP01Scope(scope))
		}
	}
	for _, scope := range scopes {
		status := AdmissionStatus{Scope: scope, State: "NONE"}
		path := filepath.Join(admissionDir(home), scope+".json")
		if !reparsePath(path) {
			if raw, err := os.ReadFile(path); err == nil {
				var entry AdmissionEntry
				if json.Unmarshal(raw, &entry) == nil && entry.Schema == AdmissionSchema {
					status.State, status.Reasons, status.CreatedAt = entry.State, entry.Reasons, entry.CreatedAt
					status.NotExercisable, status.Gaps = entry.NotExercisable, entry.Gaps
					for _, c := range entry.Cases {
						if c.Label != "" && c.Label != "PASS" {
							status.Labels = append(status.Labels, c.ID+":"+c.Label)
						}
					}
					status.Stale = "KEY_UNAVAILABLE"
					if key != nil {
						status.Stale = ""
						if _, err := LookupAdmission(home, key(scope)); err != nil {
							status.Stale = strings.TrimPrefix(err.Error(), "ADMISSION_MISS: ")
						}
					}
				}
			}
		}
		out = append(out, status)
	}
	return out
}

// AdmissionKeyFor is the cache key the gate and the probe both compute for one scope
// (admission contract r2 cache.key): the frozen contracts and restrictions come from the
// embedded contract, the rest from what is installed and running now.
func AdmissionKeyFor(scope, nativeSHA256, clauductSHA256, probeSHA256, managedFingerprint string, probeSchema int) AdmissionKey {
	return AdmissionKey{
		NativeSHA256:             nativeSHA256,
		ConfigurationID:          admissioncontract.ConfigurationID,
		RoutingContractSHA256:    admissioncontract.RoutingSHA256(),
		AdmissionContractSHA256:  admissioncontract.AdmissionSHA256(),
		ProbeSchemaRevision:      probeSchema,
		ClauductBinarySHA256:     clauductSHA256,
		ManagedSourceFingerprint: managedFingerprint,
		Scope:                    scope,
		RestrictionsDigest:       RestrictionsDigest(admissioncontract.Restrictions()),
		ProbeExecutableSHA256:    probeSHA256,
	}
}

// ManagedSourceFingerprint digests the machine policy documents the product reads (source
// and content of each, sorted), so a policy change makes every entry stale.
func ManagedSourceFingerprint(env map[string]string) (string, error) {
	docs, err := readManagedPolicy(env)
	if err != nil {
		return "", err
	}
	parts := []string{}
	for _, d := range docs {
		parts = append(parts, d.Source+"\x00"+d.JSON)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return hex.EncodeToString(sum[:]), nil
}
