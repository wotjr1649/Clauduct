package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// Managed settings outrank everything this launcher passes, --settings included, and native
// applies them again while a session runs (managed-settings, native 2.1.289). A machine
// policy that names an endpoint, a provider, a credential source or a login gateway can
// therefore move model requests off this session's gateway, and nothing Clauduct sets can
// win against it (#295). Such a policy is refused before anything starts, and a session it
// appears during is ended.
//
// Every machine source is read, not only the one native selects: native composes the env
// block per variable across admin sources, and a policyHelper replaces them all. HKCU is
// read too although native skips it when an admin source exists; refusing an inert HKCU
// value is the cost of not modelling native's source selection here.

// errPolicyUnreadable is a managed source that exists and could not be read, which is never
// taken for no policy.
var errPolicyUnreadable = errors.New("MANAGED_POLICY_UNREADABLE")

// policyDocument is one managed settings source as native reads it.
type policyDocument struct {
	Source string
	JSON   string
}

// routingPolicyKeys are top-level managed keys that decide where or how native authenticates.
var routingPolicyKeys = []string{"apiKeyHelper", "policyHelper", "forceLoginGatewayUrl", "allowedProviders"}

// routingPolicyEnv are env names that move requests or replace this session's credential.
var routingPolicyEnv = append([]string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY",
	"ANTHROPIC_CUSTOM_HEADERS", "ANTHROPIC_UNIX_SOCKET", "CLAUDE_CODE_OAUTH_TOKEN", launch.HostRoutedEnv}, launch.ProviderSwitches...)

// readManagedPolicy reads native's machine policy sources: the HKLM and HKCU registry values,
// C:\Program Files\ClaudeCode\managed-settings.json with its managed-settings.d drop-ins, and
// the same files in a directory handed to native through CLAUDE_CODE_MANAGED_SETTINGS_PATH.
func readManagedPolicy(env map[string]string) ([]policyDocument, error) {
	var docs []policyDocument
	machine, user, err := platform.ClaudePolicy()
	if err != nil {
		return nil, fmt.Errorf("%w: the Claude Code registry policy could not be read: %v", errPolicyUnreadable, err)
	}
	if machine != "" {
		docs = append(docs, policyDocument{`HKLM\SOFTWARE\Policies\ClaudeCode`, machine})
	}
	if user != "" {
		docs = append(docs, policyDocument{`HKCU\SOFTWARE\Policies\ClaudeCode`, user})
	}
	roots := []string{managedRoot()}
	var files []string
	for name, value := range env {
		switch {
		case value == "":
		case strings.EqualFold(name, "CLAUDE_CODE_MANAGED_SETTINGS_PATH"):
			roots = append(roots, value)
		case strings.EqualFold(name, "CLAUDE_CODE_REMOTE_SETTINGS_PATH"):
			// Native reads this file as its remote managed settings, in place of the fetch.
			files = append(files, value)
		}
	}
	for _, root := range roots {
		files = append(files, filepath.Join(root, "managed-settings.json"))
		// Listed, not globbed: a directory name may hold a pattern character.
		entries, err := os.ReadDir(filepath.Join(root, "managed-settings.d"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s could not be listed", errPolicyUnreadable, filepath.Join(root, "managed-settings.d"))
		}
		for _, entry := range entries { // sorted by name
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				files = append(files, filepath.Join(root, "managed-settings.d", entry.Name()))
			}
		}
	}
	for _, file := range files {
		raw, err := boundedRoleFile(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s could not be read", errPolicyUnreadable, file)
		}
		docs = append(docs, policyDocument{file, string(raw)})
	}
	return docs, nil
}

// managedRouting refuses the first document that can move model requests off this gateway.
func managedRouting(docs []policyDocument, readErr error) error {
	if readErr != nil {
		return readErr
	}
	for _, doc := range docs {
		key, ok := routingKey(doc.JSON)
		if !ok {
			return fmt.Errorf("%w: %s is not a settings object with an object env", errPolicyUnreadable, doc.Source)
		}
		if key != "" {
			return fmt.Errorf("MANAGED_ROUTING_POLICY: %s sets %s. Managed settings outrank Clauduct, so model requests could leave this session's gateway", doc.Source, key)
		}
	}
	return nil
}

// routingKey names the first routing key a settings document sets, or "" for none. It
// reports false for a document it cannot read, which the caller does not take as none.
func routingKey(document string) (string, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(document), &fields) != nil || fields == nil {
		return "", false
	}
	for _, name := range routingPolicyKeys {
		if _, set := fields[name]; set {
			return name, true
		}
	}
	var method string
	if json.Unmarshal(fields["forceLoginMethod"], &method) == nil && method == "gateway" {
		return "forceLoginMethod", true
	}
	var env map[string]any
	if raw, set := fields["env"]; set && json.Unmarshal(raw, &env) != nil {
		return "", false
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		upper := strings.ToUpper(name)
		value, _ := env[name].(string)
		switch {
		case slices.Contains(routingPolicyEnv, upper):
			return "env." + name, true
		case upper == "NO_PROXY" && !launch.NoProxyCoversGateway(value):
			// A managed NO_PROXY replaces the one that keeps the gateway off a proxy.
			return "env." + name, true
		}
	}
	return "", true
}
