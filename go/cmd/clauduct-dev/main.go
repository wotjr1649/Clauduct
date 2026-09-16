// Command clauduct-dev holds Clauduct's own commands.
//
// It is a separate binary from the product launcher so that asking Clauduct a question can
// never be confused with passing an option to the native client. clauduct --version is
// the native client's version; clauduct-dev version is this bridge's.
//
// version and doctor read no credential and open no socket: doctor exists to answer "can
// this machine even start a session" without starting one. probe is the exception and says
// so — it sends real requests, and it refuses to do anything at all without --send.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/buildinfo"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "version":
		return version(stdout)
	case "doctor":
		return doctor(stdout)
	case "probe":
		return probe(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: clauduct-dev [version|doctor|probe]")
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

// doctor reports what a session would find. It does not bind, spawn, or authenticate.
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
	fmt.Fprintln(out, "credentials  none read")
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
