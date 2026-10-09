package app

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// ResidentRecord is what a regenerated background resident needs to serve the native job a
// resident that died dispatched (#319 decision 18c). The job's settings fix its endpoint port,
// session link and events directory, so those are kept; no credential is: the regenerated
// gateway issues a new token, which native takes from its key helper after one 401/403.
// The resident writes it once the job exists and removes it on every ending it controls, so a
// record outlives only a resident that did not end itself.
type ResidentRecord struct {
	Link string `json:"link"`
	Job  string `json:"job"`
	// Session is the native session the gateway last registered; the regenerated resident
	// takes it back as a resume. Empty until the job's first hook has run.
	Session   string   `json:"session,omitempty"`
	Port      int      `json:"port"`
	PluginDir string   `json:"pluginDir,omitempty"`
	Cwd       string   `json:"cwd"`
	Args      []string `json:"args"`
}

// ResidentRestoreRole is the resident a key helper starts when its link is gone.
const ResidentRestoreRole = "--clauduct-background-restore"

var residentLink, residentJob = regexp.MustCompile(`^[a-f0-9]{32}$`), regexp.MustCompile(`^[a-f0-9]{8}$`)

func residentPath(home, link string) string {
	return filepath.Join(home, ".clauduct", "residents", link+".json")
}

func (r ResidentRecord) check() error {
	if !residentLink.MatchString(r.Link) || !residentJob.MatchString(r.Job) || r.Session != "" && !gateway.IsSessionUUID(r.Session) || r.Port < 1 || r.Port > 65535 || !filepath.IsAbs(r.Cwd) ||
		r.PluginDir != "" && (filepath.Dir(r.PluginDir) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(r.PluginDir), nativeEventsPrefix)) {
		return errors.New("RESIDENT_RECORD_INVALID")
	}
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
// gateway takes back, as a resume and never a fork; empty when no hook had registered one.
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

func removeResident(home, link string) {
	if residentLink.MatchString(link) {
		os.Remove(residentPath(home, link))
	}
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
