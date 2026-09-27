package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var ErrSessionProfile = errors.New("SESSION_PROFILE_UNVERIFIED")
var ErrSessionInUse = errors.New("SESSION_ALREADY_IN_USE")
var errSessionRestart = errors.New("SESSION_RESTART_REQUIRED")
var sessionUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func IsSessionUUID(id string) bool { return sessionUUID.MatchString(id) }

// SessionProfile is metadata only. Settings remain manual; S selections and the
// original routing preferences belong to this native session, never to defaults.
type SessionProfile struct {
	Version    int              `json:"version"`
	ID         string           `json:"id"`
	Selection  bridge.Selection `json:"selection"`
	Last       bridge.Pair      `json:"last"`
	Transcript string           `json:"transcript"`
	Offset     int64            `json:"offset"`
}

// SessionProfiles owns the OS locks for one launcher. A record stays locked
// until that launcher has reaped native and saved its final observed choice.
type SessionProfiles struct {
	mu    sync.Mutex
	root  *os.Root
	locks map[string]*os.File
}

func OpenSessionProfiles(directory string) (*SessionProfiles, error) {
	if !filepath.IsAbs(directory) || os.MkdirAll(directory, 0700) != nil {
		return nil, ErrSessionProfile
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrSessionProfile
	}
	return &SessionProfiles{root: root, locks: make(map[string]*os.File)}, nil
}

func (s *SessionProfiles) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	for id, file := range s.locks {
		err = errors.Join(err, file.Close())
		delete(s.locks, id)
	}
	err = errors.Join(err, s.root.Close())
	if err != nil {
		return ErrSessionProfile
	}
	return nil
}

// lock is called with s.mu held. Names are canonical UUIDs, excluding Windows
// device names, separators, aliases and case-insensitive filename collisions.
func (s *SessionProfiles) lock(id string) error {
	if !sessionUUID.MatchString(id) {
		return ErrSessionProfile
	}
	if s.locks[id] != nil {
		return nil
	}
	if len(s.locks) >= maxAgents {
		return ErrSessionProfile
	}
	file, err := s.root.OpenFile(id+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return ErrSessionProfile
	}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		file.Close()
		return ErrSessionProfile
	}
	if err := platform.LockFile(file); err != nil {
		file.Close()
		return ErrSessionInUse
	}
	s.locks[id] = file
	return nil
}

func (s *SessionProfiles) Load(id string) (SessionProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.lock(id); err != nil {
		return SessionProfile{}, err
	}
	file, err := s.root.Open(id + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return SessionProfile{}, os.ErrNotExist
	}
	if err != nil {
		return SessionProfile{}, ErrSessionProfile
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return SessionProfile{}, ErrSessionProfile
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return SessionProfile{}, ErrSessionProfile
	}
	return parseSessionProfile(raw, id)
}

func parseSessionProfile(raw []byte, id string) (SessionProfile, error) {
	bad := func() (SessionProfile, error) { return SessionProfile{}, ErrSessionProfile }
	fields, err := wire.Fields(raw, []string{"version", "id", "selection", "last", "transcript", "offset"})
	var profile SessionProfile
	if err != nil || len(raw) > 1<<20 || len(fields) != 6 || !sessionUUID.MatchString(id) ||
		json.Unmarshal(fields["version"], &profile.Version) != nil || profile.Version != 1 ||
		json.Unmarshal(fields["id"], &profile.ID) != nil || profile.ID != id ||
		json.Unmarshal(fields["transcript"], &profile.Transcript) != nil || string(fields["transcript"]) == "null" ||
		json.Unmarshal(fields["offset"], &profile.Offset) != nil || string(fields["offset"]) == "null" || profile.Offset < 0 {
		return bad()
	}
	if profile.Transcript != "" && (!filepath.IsLocal(profile.Transcript) || filepath.Base(profile.Transcript) != id+".jsonl" || len(profile.Transcript) > 4096) || profile.Transcript == "" && profile.Offset != 0 {
		return bad()
	}
	profile.Selection, err = bridge.ParseSelection(fields["selection"])
	if err != nil {
		return bad()
	}
	frozen := profile.Selection.Snapshot()
	if len(frozen.ModelDefaults) != len(profile.Selection.ModelDefaults) || len(frozen.ModelMapping) != len(profile.Selection.ModelMapping) || len(frozen.RoleDefaults) != len(profile.Selection.RoleDefaults) {
		return bad()
	}
	profile.Last, err = bridge.ParsePair(fields["last"])
	if err != nil {
		return bad()
	}
	return profile, nil
}

func (s *SessionProfiles) Save(profile SessionProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(profile)
	if err != nil {
		return ErrSessionProfile
	}
	if _, err := parseSessionProfile(raw, profile.ID); err != nil {
		return err
	}
	if err := s.lock(profile.ID); err != nil {
		return err
	}
	nonce, err := newToken()
	if err != nil {
		return ErrSessionProfile
	}
	temp := profile.ID + "." + nonce + ".tmp"
	file, err := s.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrSessionProfile
	}
	defer s.root.Remove(temp)
	_, writeErr := file.Write(raw)
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || s.root.Rename(temp, profile.ID+".json") != nil {
		return ErrSessionProfile
	}
	return nil
}

// Refresh reads only the recorded native path below its projects root. A partial
// append is retryable while native runs; a final/resume read must reach EOF.
func (s *SessionProfiles) Refresh(profile *SessionProfile, projects string, final bool) error {
	if profile.Transcript == "" {
		return nil
	}
	root, err := os.OpenRoot(projects)
	if errors.Is(err, os.ErrNotExist) && profile.Offset == 0 {
		return nil
	}
	if err != nil {
		return ErrSessionProfile
	}
	defer root.Close()
	file, err := root.Open(profile.Transcript)
	if errors.Is(err, os.ErrNotExist) && profile.Offset == 0 {
		return nil // Native creates a new transcript lazily.
	}
	if err != nil {
		return ErrSessionProfile
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < profile.Offset {
		return ErrSessionProfile
	}
	if profile.Offset > 0 {
		var boundary [1]byte
		if _, err := file.ReadAt(boundary[:], profile.Offset-1); err != nil || boundary[0] != '\n' {
			return ErrSessionProfile
		}
	}
	pair, offset, _, err := readSessionChoice(file, profile.ID, profile.Offset, profile.Last)
	if err != nil || final && offset != info.Size() {
		return errSessionSelection
	}
	if pair == profile.Last && offset == profile.Offset {
		return nil
	}
	next := *profile
	next.Last, next.Offset = pair, offset
	if err := s.Save(next); err != nil {
		return err
	}
	*profile = next
	return nil
}

type profileState struct {
	mu         sync.Mutex
	store      *SessionProfiles
	startup    bridge.Pair
	expected   string
	fork       bool
	background bool
	current    string
	profiles   map[string]*SessionProfile
	pending    string
	ready      bool
}

// ConfigureSessionProfiles binds immutable routing preferences before native
// starts. Choosing another saved session requires a fresh native launch with
// that snapshot; a mismatched process never sends an inference first.
func (g *Gateway) ConfigureSessionProfiles(store *SessionProfiles, startup bridge.Pair, resumed *SessionProfile, fork, background bool) {
	if g.contexts == nil || store == nil {
		return
	}
	p := &profileState{store: store, startup: startup, fork: fork, background: background, profiles: make(map[string]*SessionProfile)}
	if resumed != nil && !fork {
		copy := *resumed
		copy.Last = startup
		p.expected = copy.ID
		p.profiles[copy.ID] = &copy
	}
	g.profiles = p
}

// Caller holds contexts.mu; no profile operation takes contexts.mu in return.
func (g *Gateway) registerSessionProfile(session, source string) (err error) {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() { p.ready = err == nil }()
	if !sessionUUID.MatchString(session) {
		return ErrSessionProfile
	}
	if p.pending != "" {
		return errSessionRestart
	}
	if source == "resume" && p.current != "" {
		// Even a previously visited session can have a different last S effort.
		// Native's in-process resume did not restore it in the measured client.
		// A background worker respawns with its original flags. It can reconnect
		// only to that same session, while those flags still match its last S.
		current := p.profiles[session]
		if p.background && p.current == session && current != nil {
			if err := p.store.Refresh(current, g.delegations.projects, true); err != nil {
				return err
			}
		}
		if !p.background || p.current != session || current == nil || current.Last != p.startup {
			p.pending = session
			return errSessionRestart
		}
	}
	if current := p.profiles[session]; current != nil {
		next := *current
		next.Transcript = g.contexts.sessions[session]
		if err := p.store.Refresh(&next, g.delegations.projects, false); err != nil {
			return err
		}
		if p.current == "" || current.Transcript != next.Transcript {
			if err := p.store.Save(next); err != nil {
				return err
			}
		}
		*current = next
		p.current = session
		return nil
	}
	_, loadErr := p.store.Load(session)
	if source == "resume" && loadErr == nil {
		p.pending = session
		return errSessionRestart
	}
	if source != "startup" && source != "clear" && source != "fork" && source != "resume" {
		return ErrSessionProfile
	}
	if source == "startup" && p.expected != "" {
		return ErrSessionProfile
	}
	if !errors.Is(loadErr, os.ErrNotExist) {
		if loadErr != nil {
			return loadErr
		}
		return ErrSessionProfile // Never replace an existing session from startup/clear.
	}
	pair := p.startup
	if source == "clear" || source == "fork" && (!p.fork || p.current != "") {
		prior := p.profiles[p.current]
		if prior == nil {
			return ErrSessionProfile
		}
		if err := p.store.Refresh(prior, g.delegations.projects, true); err != nil {
			return err
		}
		pair = prior.Last
	}
	profile := &SessionProfile{Version: 1, ID: session, Selection: g.selection.Clone(), Last: pair, Transcript: g.contexts.sessions[session]}
	if source == "resume" || p.fork && p.current == "" {
		// An old session (or copied fork) starts on the pair supplied to this
		// launch. Earlier S rows must not replace that new baseline.
		root, err := g.delegations.openProjects(profile.Transcript)
		if err != nil {
			return ErrSessionProfile
		}
		defer root.Close()
		file, err := root.Open(profile.Transcript)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrSessionProfile
		}
		if err == nil {
			defer file.Close()
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return ErrSessionProfile
			}
			profile.Offset = info.Size()
			if profile.Offset > 0 {
				var last [1]byte
				if _, err := file.ReadAt(last[:], profile.Offset-1); err != nil || last[0] != '\n' {
					return ErrSessionProfile
				}
			}
		}
	}
	if err := p.store.Save(*profile); err != nil {
		return err
	}
	p.profiles[session], p.current = profile, session
	return nil
}

// CheckpointSessionProfiles also runs after the owned native process is reaped,
// so S followed immediately by exit does not depend on a model request or timer.
func (g *Gateway) CheckpointSessionProfiles(final bool) error {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var result error
	for _, profile := range p.profiles {
		result = errors.Join(result, p.store.Refresh(profile, g.delegations.projects, final))
	}
	if result != nil {
		p.ready = false
	}
	return result
}

func (g *Gateway) sessionProfileError(session string) error {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != "" {
		return errSessionRestart
	}
	if !p.ready || p.profiles[session] == nil || p.current != session {
		return ErrSessionProfile
	}
	return nil
}

func (g *Gateway) refuseSessionProfile(w http.ResponseWriter, session string, err error) {
	category, detail := ErrSessionProfile.Error(), ""
	if errors.Is(err, errSessionRestart) {
		category = errSessionRestart.Error()
		if sessionUUID.MatchString(session) {
			// Only a validated UUID enters the command, never a title or path.
			detail = "; save any draft, finish native, then run clauduct --resume " + session + " with any native options you need."
		}
	} else if errors.Is(err, ErrSessionInUse) {
		category = ErrSessionInUse.Error()
	}
	g.refuseDetail(w, refusal{category: category, status: http.StatusBadRequest}, detail)
}
