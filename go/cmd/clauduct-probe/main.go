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
	"strings"
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
	long := flag.Bool("long", false, "run the scope's RP01 check (about 75 minutes) instead of its short cases")
	only := flag.String("only", "", "comma-separated cases to run as a diagnostic; writes no entry")
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
		*out = filepath.Join(*clauductHome, ".clauduct", "provider-admission", "runs", fmt.Sprintf("%s-%s-%d", *scope, stamp, os.Getpid()))
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	root := filepath.Join(os.TempDir(), fmt.Sprintf("clauduct-probe-%s-%d", stamp, os.Getpid()))
	r, err := providerprobe.NewRun(root, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	// Every session this run starts gets a profile of its own, never the user's: native reads
	// user memory and other per-user files from it (as the private test harness does).
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 2
	}
	os.Setenv("USERPROFILE", profile)
	defer os.RemoveAll(root)
	required := map[string]string{}
	var entry app.AdmissionEntry
	err = r.Do(func() error {
		var err error
		required, err = providerprobe.RequiredStates(*clauductHome, *product, probeSHA, *scope)
		if err != nil {
			return err
		}
		options := providerprobe.ScopeOptions{Product: *product, ProbeSHA256: probeSHA, Scope: *scope,
			EvidenceDir: *out, Home: *clauductHome, Required: required, Only: onlyCases(*only)}
		if *long {
			entry, err = providerprobe.RunRP01(r, options)
			return err
		}
		entry, err = providerprobe.RunScope(r, options)
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct-probe:", err)
		return 1
	}
	fmt.Printf("scope %s %s %v (admission contract %s, evidence %s)\n", *scope, entry.State, entry.Reasons, admissioncontract.AdmissionSHA256()[:12], *out)
	return 0
}

func onlyCases(list string) []string {
	if list == "" {
		return nil
	}
	return strings.Split(list, ",")
}
