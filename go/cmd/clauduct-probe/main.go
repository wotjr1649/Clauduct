// Command clauduct-probe is Clauduct's provider-control admission runner (#320, admission
// contract r2): it measures one launch scope against the claude.exe this machine resolves and
// writes the admission entry the gate in clauduct.exe reads. It is the only writer of those
// entries; clauduct.exe runs it from its own directory, as does clauduct --dev --probe
// provider-control.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/providerprobe"
)

func main() {
	if code, ok := providerprobe.Role(os.Args); ok {
		os.Exit(code)
	}
	os.Exit(run())
}

func run() int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	home, _ := os.UserHomeDir()
	scope := flag.String("scope", app.ScopePrintSingle, "admission scope")
	product := flag.String("product", filepath.Join(filepath.Dir(self), "clauduct.exe"), "the clauduct.exe to certify")
	clauductHome := flag.String("home", home, "Clauduct home: where the entry is written")
	out := flag.String("out", "", "evidence directory (default: under the admission directory)")
	flag.Parse()

	raw, err := os.ReadFile(self)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	sum := sha256.Sum256(raw)
	probeSHA := hex.EncodeToString(sum[:])
	stamp := time.Now().UTC().Format("20060102T150405Z")
	if *out == "" {
		*out = filepath.Join(*clauductHome, ".clauduct", "provider-admission", "runs", *scope+"-"+stamp)
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	r, err := providerprobe.NewRun(filepath.Join(os.TempDir(), "clauduct-probe-"+stamp), os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	required := map[string]string{}
	var entry app.AdmissionEntry
	err = r.Do(func() error {
		var err error
		required, err = providerprobe.RequiredStates(*clauductHome, *product, probeSHA, *scope)
		if err != nil {
			return err
		}
		entry, err = providerprobe.RunScope(r, providerprobe.ScopeOptions{Product: *product, ProbeSHA256: probeSHA, Scope: *scope,
			EvidenceDir: *out, Home: *clauductHome, Required: required})
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 1
	}
	fmt.Printf("scope %s %s %v (admission contract %s, evidence %s)\n", *scope, entry.State, entry.Reasons, admissioncontract.AdmissionSHA256()[:12], *out)
	return 0
}
