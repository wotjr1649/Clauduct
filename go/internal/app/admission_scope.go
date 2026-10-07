package app

import (
	"errors"
	"regexp"
	"strings"
)

// Admission scopes (#320, admission contract r1 scopeSelection): the execution paths a
// provider-control launch can be admitted for, each verified on its own.
const (
	ScopePrintSingle    = "PRINT_SINGLE"
	ScopeResumeExplicit = "RESUME_EXPLICIT"
	ScopeContinuePrint  = "CONTINUE_PRINT"
	ScopeMultiTurn      = "MULTI_TURN_SAME_PROCESS"
	ScopeTUI            = "TUI"
)

// ErrNoAdmissionScope refuses a launch form no admission scope covers. Nothing is started.
var ErrNoAdmissionScope = errors.New("ROUTING_PROVIDER_UNVERIFIED: this launch form is not one Clauduct has verified under its provider control (a native subcommand, a one-word or several positional arguments, or an option combination outside the verified paths). Nothing was started; give the prompt as one argument with spaces, or with -p on standard input")

var sessionUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// admissionScope is the first matching rule of admission contract r1 scopeSelection for argv,
// or an error naming why there is none. Options are read the way native reads them
// (nativeArgEnd); an option this build cannot place, or combined short flags, give no scope.
func admissionScope(args []string) (string, error) {
	var positionals []string
	options := map[string]string{}
	for i := 0; i < len(args); {
		if args[i] == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			return "", ErrNoAdmissionScope
		}
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			i = end
			continue
		}
		if len(arg) > 2 && arg[1] != '-' {
			return "", ErrNoAdmissionScope // combined short flags
		}
		name, value, attached := strings.Cut(arg, "=")
		if !attached && end == i+2 {
			value = args[i+1]
		}
		switch name {
		case "-p":
			name = "--print"
		case "-r":
			name = "--resume"
		case "-c":
			name = "--continue"
		}
		options[name] = value
		i = end
	}
	has := func(name string) bool { _, ok := options[name]; return ok }
	if has("--bg") || has("--background") {
		return "", ErrBackgroundUnverified
	}
	if len(positionals) > 1 || len(positionals) == 1 && !strings.ContainsAny(positionals[0], " \t\r\n") {
		return "", ErrNoAdmissionScope
	}
	for _, name := range []string{"--remote-control", "--teleport", "--cloud", "--from-pr", "--fork-session"} {
		if has(name) {
			return "", ErrNoAdmissionScope
		}
	}
	resume, continued := has("--resume"), has("--continue")
	if resume && continued {
		return "", ErrNoAdmissionScope
	}
	print := has("--print")
	switch {
	case print && options["--input-format"] == "stream-json":
		if resume || continued {
			return "", ErrNoAdmissionScope
		}
		return ScopeMultiTurn, nil
	case print && resume:
		if !sessionUUID.MatchString(options["--resume"]) {
			return "", ErrNoAdmissionScope
		}
		return ScopeResumeExplicit, nil
	case print && continued:
		return ScopeContinuePrint, nil
	case print:
		return ScopePrintSingle, nil
	case !resume && !continued:
		return ScopeTUI, nil
	}
	return "", ErrNoAdmissionScope
}
