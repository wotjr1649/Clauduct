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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/buildinfo"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/settingsfile"
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
	case "init-settings":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: clauduct --dev --init-settings")
			return 2
		}
		home, err := os.UserHomeDir()
		if err == nil {
			err = settingsfile.Ensure(home)
		}
		if err != nil {
			fmt.Fprintln(stderr, settingsfile.ErrCreate)
			return 1
		}
		return 0
	case "sync-settings":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: clauduct --dev --sync-settings")
			return 2
		}
		return syncSettings(stdout, stderr)
	case "doctor":
		return doctor(stdout)
	case "usage":
		return usage(stdout)
	case "probe":
		return probe(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: clauduct --dev [--version|--verification-budget-version|--init-settings|--sync-settings|--doctor|--usage|--probe]")
		return 2
	}
}

// syncSettings appends the top-level keys this release's defaults have and the user's
// settings.json lacks. It reads no credential and opens no socket; installers and the
// updater run it with the newly installed binary so the defaults are that release's.
func syncSettings(stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, settingsfile.ErrSyncTarget)
		return 1
	}
	result, err := settingsfile.Sync(home)
	if err != nil {
		// The error is a fixed code and, when one was made, the backup's path.
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch {
	case result.Created:
		fmt.Fprintln(stdout, "settings created", filepath.Join(home, ".clauduct", "settings.json"))
	case len(result.Added) == 0:
		fmt.Fprintln(stdout, "settings unchanged")
	}
	if len(result.Added) > 0 {
		fmt.Fprintln(stdout, "settings added", strings.Join(result.Added, ", "))
		fmt.Fprintln(stdout, "settings backup", result.Backup)
	}
	return 0
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

	source := platform.Environment(os.Environ())
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
