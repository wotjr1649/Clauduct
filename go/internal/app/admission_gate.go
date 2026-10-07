package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// ExpectedProbeSHA256 is the SHA-256 of the clauduct-probe.exe built from the same source as
// this binary (admission r2 runner.invocation). cmd/clauduct sets it from the file embedded at
// build; empty means this build knows no probe and every gated launch is refused.
var ExpectedProbeSHA256 string

// ProbeName is the probe's file name; it is only ever run from this executable's directory.
const ProbeName = "clauduct-probe.exe"

// Gate refusals. Each ends the launch before native starts; nothing falls back to a launch
// without the strategy.
var (
	ErrProbeUnknown  = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXPECTED_UNKNOWN: this build carries no expected clauduct-probe.exe digest")
	ErrProbeMismatch = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXECUTABLE_MISMATCH: clauduct-probe.exe next to clauduct.exe is not the one this build expects")
	// An updater from a release without the probe installs clauduct.exe alone; running the
	// new clauduct.exe's own update installs the pair.
	ErrProbeMissing          = errors.New("ROUTING_PROVIDER_UNVERIFIED: PROBE_EXECUTABLE_MISSING: clauduct-probe.exe is not next to clauduct.exe; run clauduct --update (or install.ps1) once to install it. Nothing was started")
	errAdmissionInconclusive = "ROUTING_PROVIDER_UNVERIFIED: scope %s is %s (%v); nothing was started"
)

// probeRetryAfter is how old an INCONCLUSIVE entry must be before the gate runs the probe
// again (admission r2 states.INCONCLUSIVE).
const probeRetryAfter = 600 * time.Second

// GateInput is what one launch's gate reads.
type GateInput struct {
	Args []string
	Env  map[string]string
	// Home is ClauductHome: where entries are read and the probe writes them.
	Home   string
	Stderr io.Writer
	// RunProbe runs the probe for one scope; nil is RunProbe in this package.
	RunProbe func(ctx context.Context, call ProbeCall) error
}

// Admission is a gate decision: the strategy the launch runs under and the native it admitted.
type Admission struct {
	Strategy     string
	Scope        string
	NativeSHA256 string
}

// AdmitLaunch is admission r2's gate for an inference launch: it selects the scope, makes
// sure the scope and the scopes it requires have entries for what is installed now (running
// the probe when one is missing or stale), and admits only an ADMITTED scope whose RP01 entry,
// when the scope requires one, passed.
func AdmitLaunch(ctx context.Context, in GateInput) (Admission, error) {
	scope, err := admissionScope(in.Args)
	if err != nil {
		return Admission{}, err
	}
	if ExpectedProbeSHA256 == "" {
		return Admission{}, ErrProbeUnknown
	}
	run := in.RunProbe
	if run == nil {
		run = RunProbe
	}
	key, err := gateKey(in.Env)
	if err != nil {
		return Admission{}, err
	}
	order := append(slices.Clone(admissioncontract.Scopes()[scope].RequiresScope), scope)
	for _, s := range order {
		entry, err := LookupAdmission(in.Home, key(s))
		if !probeDue(entry, err) {
			continue
		}
		if in.Stderr != nil {
			fmt.Fprintf(in.Stderr, "clauduct: verifying provider control for %s with this build and native (%v); this takes a few minutes and starts nothing of yours\n", s, missReason(err, entry))
		}
		if err := run(ctx, ProbeCall{Scope: s, Home: in.Home, Stderr: in.Stderr}); err != nil {
			return Admission{}, fmt.Errorf("ROUTING_PROVIDER_UNVERIFIED: the provider-control probe for %s failed: %w", s, err)
		}
	}
	for _, s := range order {
		entry, err := LookupAdmission(in.Home, key(s))
		if err != nil {
			return Admission{}, fmt.Errorf(errAdmissionInconclusive, s, "unmeasured", err)
		}
		if entry.State != AdmissionAdmitted {
			return Admission{}, fmt.Errorf(errAdmissionInconclusive, s, entry.State, entry.Reasons)
		}
		// A scope with RP01 among its required cases is admitted only with its RP01 entry,
		// whether it is the launch's scope or one the launch's scope requires.
		if state := rp01State(in.Home, key, s); state != AdmissionAdmitted {
			return Admission{}, fmt.Errorf("ROUTING_PROVIDER_UNVERIFIED: scope %s needs its long policy-refresh check (RP01), which is %s; run clauduct --dev --probe provider-control --long once (about 75 minutes, synthetic only). Nothing was started", s, state)
		}
	}
	return Admission{Strategy: StrategyHostManaged, Scope: scope, NativeSHA256: key(scope).NativeSHA256}, nil
}

// probeDue reports whether a lookup calls for a probe run: a MISS, a stale entry, or an
// INCONCLUSIVE one older than probeRetryAfter.
func probeDue(entry AdmissionEntry, err error) bool {
	if err != nil {
		return true
	}
	return entry.State == AdmissionInconclusive && time.Since(entry.CreatedAt) > probeRetryAfter
}

func missReason(err error, entry AdmissionEntry) string {
	if err != nil {
		return err.Error()
	}
	return "INCONCLUSIVE since " + entry.CreatedAt.Format(time.RFC3339)
}

// gateKey is currentKey; a test replaces it.
var gateKey = currentKey

// CurrentAdmissionKey is the key builder the gate uses now (doctor reads entries with it).
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

// ErrAdmittedNativeChanged refuses a start whose native is not the one the gate admitted.
var ErrAdmittedNativeChanged = errors.New("ROUTING_PROVIDER_UNVERIFIED: NATIVE_CHANGED: claude.exe changed after its admission; nothing was started")

// startAdmitted starts spec through start; when want is set, spec's file is pinned first and
// must be the admitted native, and stays pinned until start returns.
func startAdmitted(start func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error), want string, spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error) {
	if want == "" {
		return start(spec, stdin, stdout, stderr)
	}
	pin, err := providercert.PinFile(spec.File)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrAdmittedNativeChanged, err)
	}
	defer pin.Close()
	if pin.Identity.SHA256 != want {
		return nil, ErrAdmittedNativeChanged
	}
	return start(spec, stdin, stdout, stderr)
}

// InformationOnly reports whether argv only asks native for help or its version: such a
// launch sends no model request and is not gated (admission r2 gate.trigger). Only the flag
// alone counts; with anything else native may read a subcommand first, so the launch is gated.
func InformationOnly(args []string) bool {
	return len(args) == 1 && slices.Contains([]string{"--help", "-h", "--version", "-v"}, args[0])
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

// ScopeState is a scope's admission state under the current key as the gate reads it: its
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
