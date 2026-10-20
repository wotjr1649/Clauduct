package app

import (
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// Only known native arities prove that an option-shaped token is a value.
// Attached unknown values are self-contained; ambiguous Desktop tokens cannot launch.
func nativeDesktop(args []string) error {
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		name, _, attached := strings.Cut(args[i], "=")
		if !known {
			if attached && strings.HasPrefix(name, "--") {
				i++
				continue
			}
			for _, token := range args[i+1:] {
				name, _, _ := strings.Cut(token, "=")
				if name == "--desktop" {
					return ErrDesktopBoundary
				}
			}
			return nil
		}
		if name == "--desktop" {
			return ErrDesktopUnsupported
		}
		if end > len(args) {
			return nil
		}
		i = end
	}
	return nil
}

// accountCommand refuses native's Claude account subcommands (auth login, auth logout,
// setup-token): Clauduct does not sign in to or out of Claude (#319 decision 17, routing r4
// LN). Native dispatches a subcommand that follows options of any kind, except after -p,
// which makes it the prompt (measured 2.1.294: `--model x mcp list` lists, `-p mcp list`
// asks the model); -p is read as printMode reads it, so a value attached to a short option
// is not mistaken for it. An unknown option without an attached value hides where the
// subcommand begins: an account command anywhere after it is refused, as nativeDesktop refuses
// --desktop there.
func accountCommand(args []string) error {
	var positionals []string
	for i := 0; i < len(args) && args[i] != "--" && len(positionals) < 2; {
		arg := args[i]
		end, known := nativeArgEnd(args, i)
		if known {
			// A variadic option takes every following value up to the next option, a
			// subcommand-shaped token among them included.
			end = launch.VariadicValuesEnd(args, i, end)
		}
		if known && end <= len(args) && len(positionals) == 0 && printMode(args[i:end]) {
			return nil
		}
		if !known {
			if _, _, attached := strings.Cut(arg, "="); attached && strings.HasPrefix(arg, "--") {
				i++
				continue
			}
			for j := i + 1; j < len(args) && args[j] != "--"; j++ {
				if args[j] == "setup-token" || args[j] == "auth" && j+1 < len(args) && (args[j+1] == "login" || args[j+1] == "logout") {
					return ErrAccountCommand
				}
			}
			break
		}
		if end > len(args) {
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
		}
		i = end
	}
	switch {
	case len(positionals) > 0 && positionals[0] == "setup-token",
		len(positionals) > 1 && positionals[0] == "auth" && (positionals[1] == "login" || positionals[1] == "logout"):
		return ErrAccountCommand
	}
	return nil
}

// Options before a subcommand may consume it as a variadic value and open a session.
func nativeControl(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "rm", "stop", "logs":
		return args[0]
	}
	return ""
}

// Native handles background dispatch before its ordinary help/version flags.
func nativeInformation(args []string) bool {
	if BackgroundRequested(args) {
		return false
	}
	for _, flag := range []string{"--help", "-h", "--version", "-v"} {
		if _, found := optionValue(args, flag); found {
			return true
		}
	}
	return false
}

// The launcher and gateway use the same native value boundaries.
func nativeArgEnd(args []string, i int) (end int, known bool) {
	return launch.NativeArgEnd(args, i)
}

// printMode reports whether argv asks native for -p/--print, alone or among combined shorts.
func printMode(args []string) bool {
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			break
		}
		arg := args[i]
		if arg == "--print" {
			return true
		}
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
			for _, c := range arg[1:] {
				if c == 'p' {
					return true
				}
				if !strings.ContainsRune("cvh", c) {
					break
				}
			}
		}
		i = end
	}
	return false
}

// expectedTools picks the re-measured core tools for this launch's mode (#300), or nil when
// argv narrows what native offers -- those are the user's choice, not a missing tool -- or
// when the session is a background one, whose worker mode was not measured. An option the
// scanner does not know hides the mode and any narrowing after it, so nothing is judged.
func expectedTools(args []string, core map[string][]string) []string {
	if BackgroundRequested(args) {
		return nil
	}
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			return nil
		}
		i = end
	}
	for _, option := range []string{"--tools", "--disallowedTools", "--disallowed-tools", "--agent", "--bare",
		"--disable-slash-commands", "--safe-mode", "--restricted"} {
		if _, found := optionValue(args, option); found {
			return nil
		}
	}
	if mode, _ := optionValue(args, "--permission-mode"); mode == "plan" {
		return nil // native leaves out the tool that enters plan mode
	}
	if printMode(args) {
		return core["print"]
	}
	return core["tui"]
}

// optionValue reports the last value argv gives a native option, read with the boundaries
// native uses, so a prompt that mentions the option is not taken for it. An unknown option
// stops the scan: past it, what looks like a name may be a value.
func optionValue(args []string, option string) (value string, found bool) {
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			break
		}
		name, attached, hasValue := strings.Cut(args[i], "=")
		if name == option {
			value, found = attached, true
			if !hasValue && end == i+2 {
				value = args[i+1]
			}
		}
		i = end
	}
	return value, found
}
