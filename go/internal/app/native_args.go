package app

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
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

// nativeControl returns the management verb (rm, stop, kill, logs) native dispatches before
// option parsing, or "". Native does that for a leading verb and, behind options, for a verb
// followed by an 8-hex id; anything else, an unknown option included, stays on the session
// path, which hands argv to native unchanged. Options whose side effects are unmeasured stay
// there too. Native starts a job for a background token first. The widened shapes (a verb
// behind options, and a leading kill) are only known for the pinned native build; for any
// other build, or when it cannot be identified, only a leading rm|stop|logs is sent, as
// before: native may parse the rest as a session, which must run on the gateway (#331).
func nativeControl(args []string) string {
	if BackgroundRequested(args) {
		return ""
	}
	verb := launch.NativeManagementVerb(args)
	if verb == "" {
		return ""
	}
	if (args[0] != verb || verb == "kill") && nativeArgvVersion() != launch.NativeDispatchVersion {
		return ""
	}
	if args[0] == verb {
		return verb
	}
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		if slices.Contains(unmeasuredBeforeVerb, name) {
			return ""
		}
	}
	return verb
}

// nativeControlIn is nativeControl for a launch with env. Inside a background worker
// (CLAUDE_CODE_SESSION_KIND=bg) native does not look past options for a verb, so a verb
// behind options is a root-parsed session there and keeps the session path.
func nativeControlIn(args []string, env map[string]string) string {
	command := nativeControl(args)
	if command == "" || args[0] == command {
		return command
	}
	for key, value := range env {
		if strings.EqualFold(key, "CLAUDE_CODE_SESSION_KIND") && value == "bg" {
			return ""
		}
	}
	return command
}

// unmeasuredBeforeVerb are options native may act on before it dispatches a verb. Only
// --settings and --managed-settings were read (#331 M0), so these keep the session path:
// the ones with a measured side effect or an alias of one (--rc is --remote-control's), and
// the ones that start or attach a remote or worktree session. Invert this into a list of
// measured options once M1 has measured more of them.
var unmeasuredBeforeVerb = []string{"--debug-file", "--teleport", "--remote-control", "--rc", "--plugin-url",
	"--remote", "--cloud", "--worktree", "-w", "--from-pr"}

// nativeArgvVersion is the pinned native version when the native this process resolves is the
// exact build the dispatch tables were read from (same SHA-256), "" otherwise: another
// build, not found, or unreadable. It runs nothing; the hash is computed once per path, and
// only when a rule would change an answer. shortcut: the cache ignores a file replaced while
// the process runs, a long-lived process would need the file identity in the key.
var nativeArgvVersion = func() string {
	resolve := argvResolveClaude.Load()
	exe, found, err := (*resolve)()
	if err != nil || !found {
		return ""
	}
	if cached, ok := argvPinned.Load(exe); ok {
		return cached.(string)
	}
	version := ""
	if id, err := providercert.Observe(exe); err == nil && id.SHA256 == launch.NativeDispatchSHA256 {
		version = launch.NativeDispatchVersion
	}
	argvPinned.Store(exe, version)
	return version
}

// argvResolveClaude is the native the argv pins judge: Run points it at Options.ResolveClaude,
// so the build judged is the build the session resolves (the probe's included).
var (
	argvResolveClaude atomic.Pointer[func() (string, bool, error)]
	argvPinned        sync.Map
)

func init() {
	resolve := platform.Resolver{}.Claude
	argvResolveClaude.Store(&resolve)
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
