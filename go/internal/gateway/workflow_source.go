package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// File access stays with native Read, including its hooks, permissions and full
// result bounds. The gateway writes only a per-call adapter and candidate paths.
type workflowSource struct {
	Session         string   `json:"session"`
	Call            string   `json:"call"`
	Paths           []string `json:"paths"`
	Trailer         string   `json:"trailer"`
	PreviousTrailer string   `json:"previousTrailer,omitempty"`
}

func (g *Gateway) ConfigureWorkflowSources(dirs []string) {
	if g.delegations != nil && len(dirs) <= 64 {
		g.delegations.workflowSourceDirs = append([]string(nil), dirs...)
	}
}
func (d *delegations) prepareWorkflowSource(scope delegationScope, call string, raw json.RawMessage) (json.RawMessage, error) {
	fields, err := wire.Fields(raw, []string{"scriptPath", "script", "name", "args", "description", "title"})
	if err != nil || d.events == "" {
		return nil, errDelegationUnverified
	}
	var path, name string
	paths := []string{}
	if fields["scriptPath"] != nil {
		if json.Unmarshal(fields["scriptPath"], &path) != nil || strings.TrimSpace(path) == "" || len(path) > 4096 || strings.ContainsAny(path, "\x00\r\n") {
			return nil, errDelegationUnverified
		}
		paths = append(paths, path)
	} else {
		if json.Unmarshal(fields["name"], &name) != nil || !correlationShape.MatchString(name) {
			return nil, errDelegationUnverified
		}
		for _, dir := range d.workflowSourceDirs {
			paths = append(paths, filepath.Join(dir, name+".js"))
		}
		if len(paths) == 0 {
			return nil, errDelegationUnverified
		}
	}
	trailer := workflowTrailer(scope, call)
	previousTrailer := ""
	d.mu.Lock()
	for _, run := range d.workflows {
		if run.Session == scope.session && path != "" && filepath.Clean(path) == filepath.Clean(run.Script) && run.origin.adapterBytes > 0 {
			candidate := workflowTrailer(run.origin.scope, run.Call)
			if len(candidate) == run.origin.adapterBytes {
				previousTrailer = candidate
			}
		}
	}
	d.mu.Unlock()
	placeholder, _ := json.Marshal(map[string]string{"script": trailer})
	if err := d.prepareWorkflow(scope, call, placeholder); err != nil {
		return nil, err
	}
	prepared := false
	defer func() {
		if !prepared {
			d.discard(scope.session, []string{call})
		}
	}()
	root, err := os.OpenRoot(d.events)
	if err != nil {
		return nil, errDelegationUnverified
	}
	defer root.Close()
	data, _ := json.Marshal(workflowSource{Session: scope.session, Call: call, Paths: paths, Trailer: trailer, PreviousTrailer: previousTrailer})
	if len(data) > 320<<10 {
		return nil, errDelegationUnverified
	}
	f, err := root.OpenFile("workflow-source-"+call+".json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errDelegationUnverified
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return nil, errDelegationUnverified
	}
	d.mu.Lock()
	origin := d.workflowCalls[delegationKey{scope.session, call}]
	origin.source = true
	origin.adapterBytes = len(trailer)
	origin.recoveryInput = append(json.RawMessage(nil), raw...)
	d.workflowCalls[delegationKey{scope.session, call}] = origin
	d.mu.Unlock()
	prepared = true
	return raw, nil
}

func (d *delegations) verifyWorkflowSource(link workflowLink, origin workflowOrigin, script []byte) bool {
	root, err := os.OpenRoot(d.events)
	if err != nil {
		return false
	}
	defer root.Close()
	raw, err := workflowRead(root, "workflow-source-"+link.Call+".json", 320<<10)
	var source workflowSource
	if err != nil || json.Unmarshal(raw, &source) != nil || source.Session != link.Session || source.Call != link.Call || source.Trailer != workflowTrailer(origin.scope, link.Call) || len(script) < len(source.Trailer) || !strings.HasSuffix(string(script), source.Trailer) {
		return false
	}
	raw, err = workflowRead(root, "workflow-read-"+link.Call+".json", 8192)
	var receipt struct{ Session, Call, Path, Digest string }
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Session != link.Session || receipt.Call != link.Call || !slices.Contains(source.Paths, receipt.Path) {
		return false
	}
	return workflowDigest(script[:len(script)-len(source.Trailer)]) == receipt.Digest
}
