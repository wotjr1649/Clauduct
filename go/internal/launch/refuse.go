package launch

import "strings"

// The two option names refused anywhere in argv, including values and after '--'.
// This deliberate over-refusal protects session permission checks; it does not
// claim to distinguish options from values. docs/v2/ARCHITECTURE.md section 4
// owns the CLI contract, including app's separate settings rewrite.
var refusedOptions = map[string]string{
	"--dangerously-skip-permissions":       reasonPermission,
	"--allow-dangerously-skip-permissions": reasonPermission,
}

const (
	reasonPermission = "it turns off permission checks for the whole session"
)

// Reason reports why an option is not forwarded.
func Reason(option string) string { return refusedOptions[option] }

// Refused reports the first option this launcher will not forward.
//
// Matching is per argument and exact, after an attached =value is stripped, and is
// case-insensitive. Exact rather than substring matters in both directions: a prompt that
// merely mentions the option by name is ordinary text and is forwarded, while an argument
// that *is* the option cannot hide behind casing or an attached value.
//
// No value tracking or '--' terminator applies to this scan. An exact name in
// either position is refused under the contract, even when native treats it as data.
func Refused(args []string) (string, bool) {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		for refused := range refusedOptions {
			if strings.EqualFold(name, refused) {
				return refused, true
			}
		}
	}
	return "", false
}
