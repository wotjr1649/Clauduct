package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// ExpectedProbeSHA256 is the SHA-256 of the clauduct-probe.exe built from the same source as
// this binary (admission r2 runner.invocation). cmd/clauduct sets it from the file embedded at
// build; empty means this build knows no probe and runs none.
var ExpectedProbeSHA256 string

// ProbeName is the probe's file name; it is only ever run from this executable's directory.
const ProbeName = "clauduct-probe.exe"

// These errors explain why the provider-control probe could not start.
var (
	ErrProbeUnknown  = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXPECTED_UNKNOWN: this build carries no expected clauduct-probe.exe digest")
	ErrProbeMismatch = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXECUTABLE_MISMATCH: clauduct-probe.exe next to clauduct.exe is not the one this build expects")
	// An updater from a release without the probe installs clauduct.exe alone; running the
	// new clauduct.exe's own update installs the pair.
	ErrProbeMissing = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXECUTABLE_MISSING: clauduct-probe.exe is not next to clauduct.exe; run clauduct --update (or install.ps1) once to install it. Nothing was started")
)

// gateKey builds the key for the current native and probe.
var gateKey = currentKey

// CurrentAdmissionKey builds keys for the admission entries shown by doctor.
func CurrentAdmissionKey(env map[string]string) (func(scope string) AdmissionKey, error) {
	return gateKey(env)
}

// currentKey returns the key builder for what is installed now: the native a session would
// resolve, this executable, the expected probe and the machine policy.
func currentKey(env map[string]string) (func(scope string) AdmissionKey, error) {
	native, err := ResolvedNativeSHA256()
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	id, err := providercert.Observe(self)
	if err != nil {
		return nil, err
	}
	managed, err := ManagedSourceFingerprint(env)
	if err != nil {
		return nil, err
	}
	return func(scope string) AdmissionKey {
		return AdmissionKeyFor(scope, native, id.SHA256, ExpectedProbeSHA256, managed, providercert.ProbeSchemaRevision)
	}, nil
}

// ProbeCall is one probe run: one scope's short cases, or its RP01 run when Long.
type ProbeCall struct {
	Scope  string
	Long   bool
	Home   string
	Stderr io.Writer
}

// RunProbe runs clauduct-probe.exe from this executable's directory for one scope. The file is
// pinned against change and its digest checked against ExpectedProbeSHA256 before it starts;
// the pin is held until it ends.
func RunProbe(ctx context.Context, call ProbeCall) error {
	if ExpectedProbeSHA256 == "" {
		return ErrProbeUnknown
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(self), ProbeName)
	pin, err := providercert.PinFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrProbeMissing
	}
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrProbeMismatch, err)
	}
	defer pin.Close()
	if pin.Identity.SHA256 != ExpectedProbeSHA256 {
		return ErrProbeMismatch
	}
	args := []string{"-scope", call.Scope, "-product", self, "-home", call.Home}
	if call.Long {
		args = append(args, "-long")
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout, cmd.Stderr = call.Stderr, call.Stderr
	process, err := childprocess.Start(cmd)
	if err != nil {
		return err
	}
	// The probe and everything it started end with ctx: childprocess owns them in a Job.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			process.Stop()
		case <-done:
		}
	}()
	return process.Wait()
}

// rp01State is scope's RP01 entry state under the current key: ADMITTED for a scope without
// RP01, "unmeasured" when there is no usable entry.
func rp01State(home string, key func(string) AdmissionKey, scope string) string {
	if !admissioncontract.NeedsRP01(scope) {
		return AdmissionAdmitted
	}
	entry, err := LookupAdmission(home, key(RP01Scope(scope)))
	if err != nil {
		return "unmeasured"
	}
	return entry.State
}

// ScopeState reports a scope's admission state under the current key from its
// short entry, and its RP01 entry when it requires one (admission r2 caseVerdict.entries.scope).
// A missing entry is "" (unmeasured).
func ScopeState(home string, key func(string) AdmissionKey, scope string) string {
	entry, err := LookupAdmission(home, key(scope))
	if err != nil {
		return ""
	}
	if entry.State != AdmissionAdmitted {
		return entry.State
	}
	switch rp := rp01State(home, key, scope); rp {
	case AdmissionAdmitted, AdmissionNotAdmitted:
		return rp
	}
	return AdmissionInconclusive
}
