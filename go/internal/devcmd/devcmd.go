// Package devcmd holds Clauduct's own commands: `clauduct --dev --version` and the rest.
//
// They sit behind --dev so that asking Clauduct a question can never be confused with
// passing an option to the native client. clauduct --version is the native client's
// version; clauduct --dev --version is this bridge's. Until v0.4.0 they were a separate
// binary, clauduct-dev, and a copy of clauduct.exe by that name still lands here (#112).
//
// version and doctor open no socket: doctor exists to answer "can this machine even start a
// session" without starting one. doctor reads the Codex credential only to say whether it is
// usable, and prints its category or expiry, never the credential. probe is the exception
// and says so — it sends real requests, and it refuses to do anything at all without --send.
package devcmd

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/buildinfo"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Run is one command. The command word is taken with or without its leading "--": the
// --dev form writes --version, and the clauduct-dev.exe copy that 0.3.x updaters still
// install is called the old way, as clauduct-dev version.
func Run(args []string, stdout, stderr io.Writer) int {
	command := ""
	if len(args) > 0 {
		command = strings.TrimPrefix(args[0], "--")
	}
	switch command {
	case "version":
		return version(stdout)
	case "verification-budget-version":
		fmt.Fprintln(stdout, upstream.VerificationBudgetVersion)
		return 0
	case "doctor":
		return doctor(stdout)
	case "usage":
		return usage(stdout)
	case "probe":
		return probe(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: clauduct --dev [--version|--verification-budget-version|--doctor|--usage|--probe]")
		return 2
	}
}

func version(out io.Writer) int {
	info := buildinfo.Read()
	fmt.Fprintf(out, "clauduct     %s\n", info.Version)
	fmt.Fprintf(out, "commit       %s\n", info.CommitOrUnknown())
	fmt.Fprintf(out, "go           %s\n", strings.TrimSpace(info.GoVersion+" "+runtime.GOOS+"/"+runtime.GOARCH))
	return 0
}

// doctor reports what a session would find. It does not bind or authenticate; the processes
// it starts are claude --version and codex --version, through the resolvers a session uses.
//
// Counts, not names. A dropped variable's name is chosen by the user and can itself carry
// information; the rule that produced the count is printed instead, which is the part a
// reader actually needs to check.
func doctor(out io.Writer) int {
	status := 0

	path, found, err := platform.Resolver{}.Claude()
	switch {
	case err != nil:
		fmt.Fprintf(out, "claude       ERROR %v\n", err)
		status = 1
	case found:
		fmt.Fprintf(out, "claude       found %s\n", path)
		installed, err := claudeVersion(path)
		versionReport(out, "claude", installed, err, gateway.ReferenceClient)
	default:
		fmt.Fprintf(out, "claude       NOT FOUND (looked for %s and each PATH entry)\n", path)
		status = 1
	}

	home, err := platform.OSUserHome()
	if err != nil {
		fmt.Fprintf(out, "home         ERROR %v\n", err)
		status = 1
	} else {
		fmt.Fprintf(out, "home         %s (OS user, not %%USERPROFILE%%)\n", home)
	}

	source := environMap()
	spec := launch.Build("", nil, source, "", launch.Overlay{BaseURL: "http://127.0.0.1:0", AuthToken: ""})
	fmt.Fprintf(out, "env          %d parent vars, %d passed to child\n", len(source), len(spec.Env))
	fmt.Fprintln(out, "env rule     drop ANTHROPIC_* and CLAUDE_CODE_OAUTH_TOKEN; everything else is inherited")
	if path, found, err := (platform.Resolver{}).Codex(); err == nil && found {
		fmt.Fprintf(out, "codex        found %s\n", path)
	}
	codex, codexErr := upstream.InstalledVersion()()
	versionReport(out, "codex", codex, codexErr, upstream.ReferenceClientVersion)
	catalogue(out, codex, codexErr)
	credential, err := (&auth.Provider{}).Credential()
	credentialReport(out, credential, err)
	return status
}

func environMap() map[string]string {
	entries := os.Environ()
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if i := strings.IndexByte(entry, '='); i > 0 {
			out[entry[:i]] = entry[i+1:]
		}
	}
	return out
}
