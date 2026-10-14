package launch

import "strings"

// NativeArgEnd shares native value boundaries between the launcher and gateway.
// Public arities follow Claude Code 2.1.286 --help; hidden entries retain the
// existing role scanner's contract. Unknown options cannot prove where
// a later option begins. This is not native option validation.
// --proactivity and --reply-on-resume follow the 2.1.300 handover argv.
// end is exclusive; a missing required value returns len(args)+1.
func NativeArgEnd(args []string, i int) (end int, known bool) {
	arg := args[i]
	if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
		// Native combines boolean shorts until the first value-taking option;
		// everything after that option belongs to its value, not to more flags.
		for j := 1; j < len(arg); j++ {
			switch arg[j] {
			case 'c', 'p', 'v', 'h':
				continue
			case 'n', 'r', 'w', 'd':
				if j+1 < len(arg) {
					return i + 1, true
				}
				if arg[j] == 'n' || i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					return i + 2, true
				}
				return i + 1, true
			default:
				return i + 1, false
			}
		}
		return i + 1, true
	}
	name, _, attached := strings.Cut(args[i], "=")
	end = i + 1
	switch name {
	case "--add-dir", "--agent", "--agents", "--append-system-prompt", "--append-system-prompt-file",
		"--append-subagent-system-prompt", "--append-subagent-system-prompt-file", "--allowedTools", "--allowed-tools",
		"--autocompact", "--betas", "--claude-md-file", "--client-data-url", "--debug-file", "--disallowedTools", "--disallowed-tools",
		"--effort", "--environment", "--fallback-model", "--file", "--input-format", "--json-schema",
		"--max-budget-usd", "--max-turns", "--mcp-config", "--model", "--name", "-n", "--output-format",
		"--permission-mode", "--permission-prompt-tool", "--permission-prompts", "--plugin-dir", "--plugin-dir-no-mcp", "--proactivity",
		"--plugin-url", "--remote-control-session-name-prefix", "--session-id", "--settings", "--setting-sources",
		"--system-prompt", "--system-prompt-file", "--system-prompt-snapshot", "--tools":
		if !attached {
			end++
		}
	case "--resume", "-r", "--worktree", "-w", "--debug", "-d", "--remote-control", "--teleport",
		"--cloud", "--from-pr", "--prompt-suggestions":
		if !attached && end < len(args) && !strings.HasPrefix(args[end], "-") {
			end++
		}
	case "-p", "--print", "-c", "--continue", "--verbose", "--strict-mcp-config", "--bare", "--safe-mode",
		"--disable-slash-commands", "--no-session-persistence", "--include-partial-messages", "--replay-user-messages",
		"--debug-to-stderr", "--mcp-debug", "--no-chrome", "--chrome", "--ide", "--fork-session", "--version", "-v",
		"--help", "-h", "--forward-subagent-text", "--ax-screen-reader", "--bg", "--background", "--brief", "--reply-on-resume",
		"--exclude-dynamic-system-prompt-sections", "--include-hook-events", "--restricted", "--tmux", "--desktop":
	default:
		return end, !strings.HasPrefix(name, "-") || name == "-"
	}
	return end, true
}
