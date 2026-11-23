package launch

import (
	"regexp"
	"slices"
	"strings"
)

// The functions here mirror native 2.1.330's argv scans (see native_dispatch_tables.go).
// They are separate from NativeArgEnd on purpose: that one follows the public --help and stops
// at options it does not know, while native's dispatch walks its own, larger option sets.

// NativeDispatchSets are the pinned option sets, for the golden table that anchors them.
func NativeDispatchSets() map[string][]string {
	return map[string][]string{"value": nativeValueOptions, "optional": nativeOptionalValueOptions,
		"variadic": nativeVariadicValueOptions, "boolean": nativeBooleanOptions, "noVerbValue": nativeNoVerbValueOptions}
}

func setOf(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, name := range list {
		set[name] = true
	}
	return set
}

var (
	valueSet      = setOf(nativeValueOptions)
	optionalSet   = setOf(nativeOptionalValueOptions)
	variadicSet   = setOf(nativeVariadicValueOptions)
	booleanSet    = setOf(nativeBooleanOptions)
	noVerbSet     = setOf(nativeNoVerbValueOptions)
	verbIDPattern = regexp.MustCompile(`^[a-f0-9]{8}$`)
	shortCluster  = regexp.MustCompile(`^-[A-Za-z]{2,}$`)
)

func optionShaped(arg string) bool { return len(arg) > 1 && strings.HasPrefix(arg, "-") }

// skipOption returns the index of the last token that option args[i] owns.
func skipOption(args []string, i int) int {
	name := args[i]
	if optionalSet[name] {
		if i+1 < len(args) && !optionShaped(args[i+1]) {
			return i + 1
		}
		return i
	}
	if !valueSet[name] {
		return i
	}
	end := i + 1
	if variadicSet[name] {
		for end+1 < len(args) && !optionShaped(args[end+1]) {
			end++
		}
	}
	return end
}

// NativeBackgroundFlag skips option values and reports an exact background token before the first
// "--" not consumed as an option value. The scan includes tokens after the first operand, so `prompt --bg`
// counts. Native may handle the request in the fast path or defer it to the root parser.
func NativeBackgroundFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			return false
		}
		if args[i] == "--bg" || args[i] == "--background" {
			return true
		}
		i = skipOption(args, i)
	}
	return false
}

// NativeManagementVerb is native's backgroundSessionVerbIndex restricted to the verbs the
// launcher sends straight to native (rm, stop, kill, logs): it returns the verb, or "" when
// native would not dispatch one first. A verb leading argv always counts. Behind options it
// counts only when an 8-hex id follows it, it does not directly follow "--", and every option
// before it is a known one that is not -p/--print/--bg/--background/-h/-v/--version/--routine
// or a combined short cluster. attach and respawn dispatch the same way but stay on the
// session path, and so does anything ambiguous.
func NativeManagementVerb(args []string) string {
	at := -1
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			if i+1 < len(args) {
				at = i + 1
			}
			break
		}
		if !optionShaped(args[i]) {
			at = i
			break
		}
		i = skipOption(args, i)
	}
	if at < 0 || !slices.Contains([]string{"rm", "stop", "kill", "logs"}, args[at]) {
		return ""
	}
	if at == 0 {
		return args[0]
	}
	if args[at-1] == "--" || at+1 >= len(args) || !verbIDPattern.MatchString(args[at+1]) {
		return ""
	}
	for i := 0; i < at; i = skipOption(args, i) + 1 {
		if !knownBeforeVerb(args[i]) {
			return ""
		}
	}
	return args[at]
}

// knownBeforeVerb reports whether arg may precede a management verb.
func knownBeforeVerb(arg string) bool {
	switch arg {
	case "-p", "--print", "--bg", "--background", "--routine", "-h", "--help", "-v", "-V", "--version":
		return false
	}
	if strings.HasPrefix(arg, "--routine=") || shortCluster.MatchString(arg) {
		return false
	}
	if booleanSet[arg] {
		return true
	}
	name, _, _ := strings.Cut(arg, "=")
	return valueSet[name] && !noVerbSet[name] || optionalSet[name]
}
