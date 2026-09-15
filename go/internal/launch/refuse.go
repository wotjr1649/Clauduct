package launch

import "strings"

// The only native options this launcher refuses to forward.
//
// The list is two entries because pass-through is the absence of code: refusing anything
// means building the parser the no-parser design deliberately does not have, and the Node
// baseline's own history shows where that leads — its blocklist grew to thirty names and
// it still confused `--name --model`, because it had to track which options consume a
// following value. These two consume none, so recognising them needs no such tracking and
// the property that a value can never be mistaken for an option survives intact.
//
// Everything else the baseline refused is forwarded: --mcp-config, --plugin-dir,
// --worktree, --permission-mode, --restricted, --betas, --prompt-suggestions and the rest.
// Those are the user's own configuration and letting them work is the point of the
// redesign. These two are different in kind: they remove a safety control for the whole
// session, and nothing downstream can put it back.
var refusedOptions = []string{
	"--dangerously-skip-permissions",
	"--allow-dangerously-skip-permissions",
}

// Refused reports the first option this launcher will not forward.
//
// Matching is per argument and exact, after an attached =value is stripped, and is
// case-insensitive. Exact rather than substring matters in both directions: a prompt that
// merely mentions the option by name is ordinary text and is forwarded, while an argument
// that *is* the option cannot hide behind casing or an attached value.
//
// The one place this over-refuses is an option value that happens to be exactly one of
// these names, such as --append-system-prompt --dangerously-skip-permissions. That is
// accepted. The alternative, stopping the scan at a bare "--", would under-refuse if the
// native parser still honours options after it, which has not been verified here. Over-
// refusal announces itself and the user can reword; under-refusal is silent and removes a
// permission check. The safe direction is the one that complains.
func Refused(args []string) (string, bool) {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		for _, refused := range refusedOptions {
			if strings.EqualFold(name, refused) {
				return refused, true
			}
		}
	}
	return "", false
}
