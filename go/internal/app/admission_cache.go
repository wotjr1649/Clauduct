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
)

// Admission states (admission contract r1 states).
const (
	AdmissionAdmitted     = "ADMITTED"
	AdmissionNotAdmitted  = "NOT_ADMITTED"
	AdmissionInconclusive = "INCONCLUSIVE"
)

// admissionKey is the cache key of one scope's admission (admission contract r1 cache.key).
// Every field must match exactly; any difference is a MISS with its stale reason.
type admissionKey struct {
	NativeSHA256             string `json:"nativeSHA256"`
	ConfigurationID          string `json:"configurationId"`
	RoutingContractSHA256    string `json:"routingContractSHA256"`
	AdmissionContractSHA256  string `json:"admissionContractSHA256"`
	ProbeSchemaRevision      int    `json:"probeSchemaRevision"`
	ClauductBinarySHA256     string `json:"clauductBinarySHA256"`
	ManagedSourceFingerprint string `json:"managedSourceFingerprint"`
	Scope                    string `json:"scope"`
	RestrictionsDigest       string `json:"restrictionsDigest"`
}

// admissionCase is one case's result in an entry.
type admissionCase struct {
	ID      string   `json:"id"`
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons,omitempty"`
}

// admissionEntry is one scope's cached admission.
type admissionEntry struct {
	Schema         string          `json:"schema"`
	Key            admissionKey    `json:"key"`
	State          string          `json:"state"`
	Reasons        []string        `json:"reasons,omitempty"`
	Cases          []admissionCase `json:"cases"`
	NotExercisable []string        `json:"notExercisable,omitempty"`
	Gaps           []string        `json:"gaps,omitempty"`
	EvidenceDigest string          `json:"evidenceDigest"`
	CreatedAt      time.Time       `json:"createdAt"`
	Probe          string          `json:"probeImplementation"`
}

const admissionSchema = "clauduct.provider-admission/1"

// restrictionsDigest is the SHA-256 of the restriction names, sorted and joined by newlines.
func restrictionsDigest(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

// admissionDir is where entries live. A reparse point anywhere on it is not used.
func admissionDir(home string) string { return filepath.Join(home, ".clauduct", "provider-admission") }

// errAdmissionMiss is a lookup that found no usable entry; its text is the stale reason.
type errAdmissionMiss struct{ reason string }

func (e errAdmissionMiss) Error() string { return "ADMISSION_MISS: " + e.reason }

// lookupAdmission returns the entry for key's scope when its key matches exactly. Anything
// else is a MISS: no file, an unreadable or unknown file, a reparse point, or a key that
// differs (the first differing field names the stale reason).
func lookupAdmission(home string, key admissionKey) (admissionEntry, error) {
	path := filepath.Join(admissionDir(home), key.Scope+".json")
	if reparsePath(path) {
		return admissionEntry{}, errAdmissionMiss{"REPARSE_POINT"}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return admissionEntry{}, errAdmissionMiss{"ABSENT"}
	}
	var entry admissionEntry
	if json.Unmarshal(raw, &entry) != nil || entry.Schema != admissionSchema {
		return admissionEntry{}, errAdmissionMiss{"UNREADABLE"}
	}
	switch got := entry.Key; {
	case got.NativeSHA256 != key.NativeSHA256:
		return entry, errAdmissionMiss{"NATIVE_CHANGED"}
	case got.ConfigurationID != key.ConfigurationID:
		return entry, errAdmissionMiss{"CONFIGURATION_CHANGED"}
	case got.RoutingContractSHA256 != key.RoutingContractSHA256:
		return entry, errAdmissionMiss{"ROUTING_CONTRACT_CHANGED"}
	case got.AdmissionContractSHA256 != key.AdmissionContractSHA256:
		return entry, errAdmissionMiss{"ADMISSION_CONTRACT_CHANGED"}
	case got.ProbeSchemaRevision != key.ProbeSchemaRevision:
		return entry, errAdmissionMiss{"PROBE_SCHEMA_CHANGED"}
	case got.ClauductBinarySHA256 != key.ClauductBinarySHA256:
		return entry, errAdmissionMiss{"CLAUDUCT_BINARY_CHANGED"}
	case got.ManagedSourceFingerprint != key.ManagedSourceFingerprint:
		return entry, errAdmissionMiss{"MANAGED_SOURCES_CHANGED"}
	case got.RestrictionsDigest != key.RestrictionsDigest:
		return entry, errAdmissionMiss{"RESTRICTIONS_CHANGED"}
	case got.Scope != key.Scope:
		return entry, errAdmissionMiss{"UNREADABLE"}
	}
	return entry, nil
}

// writeAdmission stores entry for its scope: a temporary file in the same directory, then a
// rename, so a reader sees the old entry or the new one and never a part.
func writeAdmission(home string, entry admissionEntry) error {
	dir := admissionDir(home)
	if reparsePath(dir) {
		return errors.New("CACHE_WRITE_FAILED: the admission directory is a reparse point")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entry.Schema = admissionSchema
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

// AdmissionStatus is what doctor shows for one scope: the stored state and whether its
// verification identity is the native found now. It never runs a probe.
type AdmissionStatus struct {
	Scope          string
	State          string // ADMITTED, NOT_ADMITTED, INCONCLUSIVE, or NONE when nothing is stored
	Reasons        []string
	NativeMatches  bool
	CreatedAt      time.Time
	NotExercisable []string
	Gaps           []string
}

// AdmissionStatuses reads each scope's stored entry under home. nativeSHA256 is the native
// found now ("" when unknown).
func AdmissionStatuses(home, nativeSHA256 string) []AdmissionStatus {
	out := []AdmissionStatus{}
	for _, scope := range AdmissionScopes {
		status := AdmissionStatus{Scope: scope, State: "NONE"}
		path := filepath.Join(admissionDir(home), scope+".json")
		if !reparsePath(path) {
			if raw, err := os.ReadFile(path); err == nil {
				var entry admissionEntry
				if json.Unmarshal(raw, &entry) == nil && entry.Schema == admissionSchema {
					status.State, status.Reasons, status.CreatedAt = entry.State, entry.Reasons, entry.CreatedAt
					status.NativeMatches = nativeSHA256 != "" && entry.Key.NativeSHA256 == nativeSHA256
					status.NotExercisable, status.Gaps = entry.NotExercisable, entry.Gaps
				}
			}
		}
		out = append(out, status)
	}
	return out
}
