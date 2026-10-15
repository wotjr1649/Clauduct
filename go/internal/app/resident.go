package app

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// ResidentRecord is what a regenerated background resident needs to serve the native job a
// resident that died dispatched (#319 decision 18c). The job's settings fix its endpoint port,
// session link and events directory, so those are kept; no credential is: the regenerated
// gateway issues a new token, which native takes from its key helper after one 401/403.
// The resident writes it once the job exists; stopped jobs keep it until native removes them.
type ResidentRecord struct {
	Link string `json:"link"`
	Job  string `json:"job"`
	// Session is Job's native session from its state; the regenerated resident resumes it.
	// Empty while the job state has no valid session ID.
	Session   string   `json:"session,omitempty"`
	Port      int      `json:"port"`
	PluginDir string   `json:"pluginDir,omitempty"`
	Cwd       string   `json:"cwd"`
	Config    string   `json:"config,omitempty"`
	Args      []string `json:"args"`
	// AbsentObserved is used only if M1 requires absence across separate executions.
	AbsentObserved bool `json:"absentObserved,omitempty"`
	// Jobs are the other native jobs on this link: dispatched from its agent view, they share
	// its settings and so its gateway, which lives until the last of them ends.
	Jobs []string `json:"jobs,omitempty"`
	// Proved contains only native identities that directly requested this gateway's models.
	Proved []gateway.NativeProcess `json:"proved,omitempty"`
}

// ResidentRestoreRole is the resident a key helper starts when its link is gone.
const ResidentRestoreRole = "--clauduct-background-restore"

// ResidentRelayRole starts a restore resident and exits before native can kill the hook tree.
const ResidentRelayRole = "--clauduct-background-relay"

var residentLink, residentJob = regexp.MustCompile(`^[a-f0-9]{32}$`), regexp.MustCompile(`^[a-f0-9]{8}$`)

func residentPath(home, link string) string {
	return filepath.Join(home, ".clauduct", "residents", link+".json")
}

func (r ResidentRecord) check() error {
	if !residentLink.MatchString(r.Link) || !residentJob.MatchString(r.Job) || r.Session != "" && !gateway.IsSessionUUID(r.Session) || r.Port < 1 || r.Port > 65535 || !filepath.IsAbs(r.Cwd) ||
		r.Config != "" && !filepath.IsAbs(r.Config) || r.PluginDir != "" && !eventsDirValid(r.PluginDir) || len(r.Proved) > gateway.MaxRoutingProofs {
		return errors.New("RESIDENT_RECORD_INVALID")
	}
	for _, job := range r.Jobs {
		if !residentJob.MatchString(job) {
			return errors.New("RESIDENT_RECORD_INVALID")
		}
	}
	for _, p := range r.Proved {
		if p.PID == 0 || p.Created <= 0 {
			return errors.New("RESIDENT_RECORD_INVALID")
		}
	}
	return nil
}

func restoreResidentProof(gw *gateway.Gateway, link string, r ResidentRecord) error {
	if r.Link != link || residentPort(gw.Addr()) != r.Port {
		return errors.New("RESIDENT_ROUTING_PROOF_MISMATCH")
	}
	gw.RestoreRoutingProof(r.Proved)
	return nil
}

// residentArgs drops the prompt: the regenerated resident starts no native process, and the
// options alone decide its configuration. With an option this build does not know, where the
// prompt begins is unknown and the arguments are kept as given.
func residentArgs(args []string) []string {
	kept := []string{}
	for i := 0; i < len(args); {
		if args[i] == "--" {
			break
		}
		end, known := nativeArgEnd(args, i)
		if !known {
			return append([]string(nil), args...)
		}
		if end > len(args) {
			end = len(args)
		}
		if strings.HasPrefix(args[i], "-") && args[i] != "-" {
			kept = append(kept, args[i:end]...)
		}
		i = end
	}
	return kept
}

// regenerateCheck admits a regeneration before anything is fetched or bound: a valid record,
// a way to bind its port, and a background launch. It returns the session the regenerated
// gateway takes back, as a resume and never a fork; empty when the job had no valid session.
func regenerateCheck(o Options, home string) (string, error) {
	if err := o.Regenerate.check(); err != nil {
		return "", err
	}
	if home == "" || o.Settings != nil || !BackgroundRequested(o.Args) {
		return "", errors.New("RESIDENT_RECORD_INVALID")
	}
	return o.Regenerate.Session, nil
}

// residentPort is the loopback port addr (host:port) names, or 0.
func residentPort(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}

func writeResident(home string, r ResidentRecord) error {
	if err := r.check(); err != nil {
		return err
	}
	path := residentPath(home, r.Link)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// Keep the old job identities until native's listing proves they can be reclaimed.
func (p *backgroundProcess) checkpointForeground() error {
	jobs := nativeJobsWithLink(p.config, p.record.Link)
	if len(jobs) == 0 && p.record.Job == "" {
		return nil
	}
	record := *p.record
	record.Jobs = slices.Clone(record.Jobs)
	if record.Job == "" {
		record.Job = jobs[0]
	}
	for _, job := range jobs {
		if job != record.Job && !slices.Contains(record.Jobs, job) {
			record.Jobs = append(record.Jobs, job)
		}
	}
	slices.Sort(record.Jobs)
	record.Session, record.Proved = nativeJobSession(p.config, record.Job), p.proved()
	if record.Job == p.record.Job && record.Session == p.record.Session && slices.Equal(record.Jobs, p.record.Jobs) && slices.Equal(record.Proved, p.record.Proved) {
		return nil
	}
	if err := writeResident(p.home, record); err != nil {
		return errors.New("FOREGROUND_RESIDENT_WRITE_FAILED")
	}
	*p.record, p.job = record, record.Job
	return nil
}

func removeResident(home, link string) error {
	if !residentLink.MatchString(link) {
		return errors.New("RESIDENT_RECORD_INVALID")
	}
	path := residentPath(home, link)
	// Keep the identity when its temporary companion cannot be removed.
	for _, name := range []string{path + ".tmp", path} {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// LoadResident reads the record for link from home.
func LoadResident(home, link string) (ResidentRecord, error) {
	if !residentLink.MatchString(link) {
		return ResidentRecord{}, errors.New("RESIDENT_RECORD_INVALID")
	}
	raw, err := os.ReadFile(residentPath(home, link))
	if err != nil {
		return ResidentRecord{}, err
	}
	var r ResidentRecord
	if err := json.Unmarshal(raw, &r); err != nil || r.Link != link {
		return ResidentRecord{}, errors.New("RESIDENT_RECORD_INVALID")
	}
	return r, r.check()
}

// RegenerableJob names the job a background resident that is gone left a record for under
// link: one native's key helper regenerates on the job's next model request.
func RegenerableJob(link string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	r, err := LoadResident(home, link)
	return r.Job, err == nil
}

func (r ResidentRecord) ownsSession(config, session string) bool {
	if !gateway.IsSessionUUID(session) {
		return false
	}
	for _, job := range append([]string{r.Job}, r.Jobs...) {
		if nativeJobSession(config, job) == session {
			return true
		}
	}
	return false
}

func nativeJobSession(config, job string) string {
	if config == "" || !residentJob.MatchString(job) {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(config, "jobs", job, "state.json"))
	var state struct {
		Session string `json:"sessionId"`
	}
	if err != nil || json.Unmarshal(raw, &state) != nil || !gateway.IsSessionUUID(state.Session) {
		return ""
	}
	return state.Session
}

func (p *backgroundProcess) releaseUnownedProfiles() error {
	if p.record == nil || p.releaseProfiles == nil {
		return nil
	}
	var keep []string
	for _, job := range append([]string{p.record.Job}, p.record.Jobs...) {
		session := nativeJobSession(p.config, job)
		if session == "" {
			return nil // A missing or partial job state proves no session safe to release.
		}
		keep = append(keep, session)
	}
	return p.releaseProfiles(keep)
}
