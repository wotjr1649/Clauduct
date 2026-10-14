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
var errSessionStartUnregistered = errors.New("SESSION_START_UNREGISTERED")
var ErrSessionInUse = errors.New("SESSION_ALREADY_IN_USE")
var errSessionRestart = errors.New("SESSION_RESTART_REQUIRED")
var sessionUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func IsSessionUUID(id string) bool { return sessionUUID.MatchString(id) }

// SessionProfile is metadata only. Settings remain manual; S selections and the
// original routing preferences belong to this native session, never to defaults.
type SessionProfile struct {
	Version    int                     `json:"version"`
	ID         string                  `json:"id"`
	Selection  bridge.Selection        `json:"selection"`
	Last       bridge.Pair             `json:"last"`
	Transcript string                  `json:"transcript"`
	Offset     int64                   `json:"offset"`
	Calls      map[string]toolIssuance `json:"calls,omitempty"`
	Parent     string                  `json:"parent,omitempty"`
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
	fields, err := wire.Fields(raw, []string{"version", "id", "selection", "last", "transcript", "offset", "calls", "parent"})
	var profile SessionProfile
	if err != nil || len(raw) > 1<<20 || (len(fields) < 6 || len(fields) > 8) || !sessionUUID.MatchString(id) ||
		json.Unmarshal(fields["version"], &profile.Version) != nil || (profile.Version != 1 && profile.Version != 2) ||
		json.Unmarshal(fields["id"], &profile.ID) != nil || profile.ID != id ||
		json.Unmarshal(fields["transcript"], &profile.Transcript) != nil || string(fields["transcript"]) == "null" ||
		json.Unmarshal(fields["offset"], &profile.Offset) != nil || string(fields["offset"]) == "null" || profile.Offset < 0 {
		return bad()
	}
	if profile.Version == 1 && len(fields) != 6 {
		return bad()
	}
	if raw, present := fields["parent"]; present {
		if json.Unmarshal(raw, &profile.Parent) != nil || !sessionUUID.MatchString(profile.Parent) || profile.Parent == id {
			return bad()
		}
	}
	if raw, present := fields["calls"]; present {
		if string(raw) == "null" || json.Unmarshal(raw, &profile.Calls) != nil || len(profile.Calls) > 4096 {
			return bad()
		}
		entries, e := wire.Fields(raw, nil)
		if e != nil {
			return bad()
		}
		for native, record := range profile.Calls {
			members, e := wire.Fields(entries[native], []string{"session", "source", "backend", "name", "generation", "agent", "turn", "sequence", "step"})
			for _, value := range members {
				if string(value) == "null" {
					return bad()
				}
			}
			if e != nil || len(members) != 9 || !validToolIssuance(native, record, id) {
				return bad()
			}
		}
	}
	if profile.Transcript != "" && (!filepath.IsLocal(profile.Transcript) || filepath.Base(profile.Transcript) != id+".jsonl" || len(profile.Transcript) > 4096) || profile.Transcript == "" && profile.Offset != 0 {
		return bad()
	}
	profile.Selection, err = bridge.ParseSelection(fields["selection"])
	if err != nil {
		return bad()
	}
	// The saved snapshot is checked for shape only. Its entry count is not compared with
	// today's lists: an account that offers one more model must not refuse an old profile.
	// Whether the saved choice is still available is the resuming session's check.
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

var afterProfileStat func(*os.File) // Test barrier; nil in the product.

// Refresh reads only the recorded native path below its projects root. A partial
// append is retryable while native runs; a final/resume read must reach EOF.
func (s *SessionProfiles) Refresh(profile *SessionProfile, projects string, final bool, selection bridge.Selection) error {
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
	if afterProfileStat != nil {
		afterProfileStat(file)
	}
	pair, offset, _, err := readSessionChoice(file, profile.ID, profile.Offset, profile.Last, selection)
	if err != nil || final && offset < info.Size() {
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

// profileState is per native process, not per gateway (#319 decision 18c): one launch can
// run several native processes at once (a /fork copy, a dispatched job, a respawned worker),
// each with its own current session. A hook names its native process; one that does not
// (an older hook) is process 0, which is the single-session behaviour this replaced.
type profileState struct {
	mu          sync.Mutex
	store       *SessionProfiles
	startup     bridge.Pair
	expected    string
	fork        bool
	background  bool
	profiles    map[string]*SessionProfile
	slots       map[processKey]string // native process -> its current session
	owner       map[string]processKey // session -> the native process that last registered it
	pending     map[processKey]string // native process -> the session it must restart into
	ready       map[string]bool       // session -> its last registration succeeded
	last        string                // the most recent registration
	forkHistory *SessionProfile
	// failedStart is the last start registration a native process failed. The hook repeats it
	// without its source before each prompt; that repeat retries it as the original event.
	failedStart map[processKey]struct{ session, source string }
}

// serving reports whether session is the current session of its native process and ready.
// Caller holds p.mu.
func (p *profileState) serving(session string) bool {
	owner, known := p.owner[session]
	return known && p.ready[session] && p.slots[owner] == session
}

// ConfigureSessionProfiles binds immutable routing preferences before native
// starts. Choosing another saved session requires a fresh native launch with
// that snapshot; a mismatched process never sends an inference first.
func (g *Gateway) ConfigureSessionProfiles(store *SessionProfiles, startup bridge.Pair, resumed *SessionProfile, fork, background bool) {
	if g.contexts == nil {
		return
	}
	if resumed != nil && !fork {
		g.nativeEvents.resumedSession = resumed.ID
		g.nativeEvents.resumedHistory = cloneToolIssuances(resumed.Calls)
		if store == nil {
			g.nativeEvents.issued = map[string]map[string]toolIssuance{resumed.ID: cloneToolIssuances(resumed.Calls)}
		}
	}
	if store == nil {
		return
	}
	p := &profileState{store: store, startup: startup, fork: fork, background: background, profiles: make(map[string]*SessionProfile),
		slots: make(map[processKey]string), owner: make(map[string]processKey), pending: make(map[processKey]string), ready: make(map[string]bool),
		failedStart: make(map[processKey]struct{ session, source string })}
	if resumed != nil && fork {
		copy := *resumed
		copy.Calls = cloneToolIssuances(resumed.Calls)
		p.forkHistory = &copy
	}
	if resumed != nil && !fork {
		copy := *resumed
		copy.Calls = cloneToolIssuances(resumed.Calls)
		copy.Last = startup
		p.expected = copy.ID
		p.profiles[copy.ID] = &copy
	}
	g.profiles = p
}

// Caller holds contexts.mu; no profile operation takes contexts.mu in return.
// process is the native process the registering hook runs under: an OS fact when native
// identity is required, otherwise what the hook said, the zero key when it said nothing.
func (g *Gateway) registerSessionProfile(session, source string, process processKey) (err error) {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if failed := p.failedStart[process]; source == "" && p.profiles[session] == nil && failed.session == session {
		source = failed.source
	}
	defer func() {
		if source != "" && err != nil && !errors.Is(err, errSessionRestart) && p.profiles[session] == nil {
			if _, exists := p.failedStart[process]; exists || len(p.failedStart) < maxAgents {
				p.failedStart[process] = struct{ session, source string }{session, source}
			}
		}
	}()
	// The session this native process is on now, "" before its first registration.
	held := p.slots[process]
	// A registration for a session its native process has already left, other than a resume,
	// is a late event bound to an earlier state (a slow hook of that session). It is refused
	// and leaves every current session exactly as it was, readiness included. A resume of a
	// left session still goes to the restart rule below. A session another native process is
	// on (a /fork copy's original) has not been left.
	if known := p.profiles[session]; known != nil && held != session && !p.serving(session) && held != "" && source != "resume" {
		return ErrSessionProfile
	}
	defer func() { p.ready[session] = err == nil }()
	if !sessionUUID.MatchString(session) {
		return ErrSessionProfile
	}
	if p.pending[process] != "" {
		return errSessionRestart
	}
	// The launch's own first registration (its --resume session) is not a transition.
	if source == "resume" && (held != "" || p.profiles[session] != nil && len(p.slots) != 0) {
		// Even a previously visited session can have a different last S effort.
		// Native's in-process resume did not restore it in the measured client.
		// A background worker respawns with its original flags, as a new native process. It
		// can reconnect only to that same session, while those flags still match its last S.
		// A native process that had no session yet resuming a known one is a worker woken or
		// respawned with its original flags, not an in-process resume: under a foreground launch
		// too (a /background job), once the hook names its process.
		known := p.profiles[session]
		reconnect := known != nil && (held == "" || p.background && held == session)
		if reconnect {
			if err := p.store.Refresh(known, g.delegations.projects, true, g.selection); err != nil {
				return err
			}
		}
		if !reconnect || known.Last != p.startup {
			p.pending[process] = session
			return errSessionRestart
		}
	}
	if known := p.profiles[session]; known != nil {
		next := *known
		next.Transcript = g.contexts.sessions[session]
		if err := p.store.Refresh(&next, g.delegations.projects, false, g.selection); err != nil {
			return err
		}
		if held == "" || known.Transcript != next.Transcript {
			if err := p.store.Save(next); err != nil {
				return err
			}
		}
		*known = next
		p.take(process, session)
		return nil
	}
	_, loadErr := p.store.Load(session)
	if source == "resume" && loadErr == nil {
		p.pending[process] = session
		return errSessionRestart
	}
	if source != "startup" && source != "clear" && source != "fork" && source != "resume" {
		if source == "" {
			return errSessionStartUnregistered
		}
		return ErrSessionProfile
	}
	// The launch was for the expected session: a different one cannot be its first. A later
	// native process of the same launch (a dispatched job) may start its own.
	first := len(p.slots) == 0
	if source == "startup" && p.expected != "" && first {
		return ErrSessionProfile
	}
	if !errors.Is(loadErr, os.ErrNotExist) {
		if loadErr != nil {
			return loadErr
		}
		return ErrSessionProfile // Never replace an existing session from startup/clear.
	}
	parent := held
	var forkParent *SessionProfile
	if source == "fork" || p.fork && first {
		// Parent proof is the launch's resume target, this native's registered session, or its sole live candidate.
		if p.fork && first && p.forkHistory != nil {
			forkParent = p.forkHistory
		} else if parent != "" && p.serving(parent) {
			forkParent = p.profiles[parent]
		} else if parent == "" {
			for id, candidate := range p.profiles {
				if !p.serving(id) || p.pending[p.owner[id]] != "" {
					continue
				}
				if forkParent != nil {
					return ErrSessionProfile
				}
				forkParent = candidate
			}
		}
		if forkParent == nil && !(p.fork && first) {
			return ErrSessionProfile
		}
	}
	pair := p.startup
	if source == "clear" || source == "fork" && (!p.fork || !first) {
		prior := p.profiles[parent]
		if source == "fork" {
			prior = forkParent
		}
		if prior == nil {
			return ErrSessionProfile
		}
		if err := p.store.Refresh(prior, g.delegations.projects, true, g.selection); err != nil {
			return err
		}
		pair = prior.Last
	}
	profile := &SessionProfile{Version: 1, ID: session, Selection: g.selection.Clone(), Last: pair, Transcript: g.contexts.sessions[session]}
	if source == "resume" || p.fork && first {
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
	if source == "fork" || p.fork && first {
		if prior := forkParent; prior != nil {
			profile.Version = 2
			profile.Parent = prior.ID
			profile.Calls = cloneToolIssuances(prior.Calls)
			for id, record := range profile.Calls {
				record.Session = session
				profile.Calls[id] = record
			}
		}
	}
	if err := p.store.Save(*profile); err != nil {
		return err
	}
	p.profiles[session] = profile
	p.take(process, session)
	return nil
}

// take makes session the current one of native process, which leaves the one it was on; a
// session that moved to this process leaves its earlier one. Caller holds p.mu.
func (p *profileState) take(process processKey, session string) {
	delete(p.failedStart, process)
	if earlier, moved := p.owner[session]; moved && earlier != process && p.slots[earlier] == session {
		delete(p.slots, earlier)
	}
	p.slots[process], p.owner[session], p.last = session, process, session
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
		result = errors.Join(result, p.store.Refresh(profile, g.delegations.projects, final, g.selection))
	}
	if result != nil {
		for session := range p.ready {
			p.ready[session] = false
		}
	}
	return result
}

// from, when native identity is required, is the native process that sent the request: the
// session must be bound to it, whatever another process registered.
func (g *Gateway) sessionProfileError(session string, from *processKey) error {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// A native process that must restart sends nothing more, for any of its sessions, and
	// neither does the session it must restart into; other native processes are unaffected.
	if owner, known := p.owner[session]; known && p.pending[owner] != "" {
		return errSessionRestart
	}
	for _, waiting := range p.pending {
		if waiting == session {
			return errSessionRestart
		}
	}
	if p.profiles[session] == nil || !p.serving(session) || from != nil && p.owner[session] != *from {
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
	} else if errors.Is(err, errSessionStartUnregistered) {
		category = errSessionStartUnregistered.Error()
	}
	g.refuseDetail(w, refusal{category: category, status: http.StatusBadRequest}, detail)
}

// CurrentSession is the native session this gateway's profiles last registered, or "". A
// background resident records it so a regenerated one can take the session back as a resume
// (#319 decision 18c).
func (g *Gateway) CurrentSession() string {
	p := g.profiles
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

// Sessions reports every session a native process has registered here.
func (g *Gateway) Sessions() []string {
	p := g.profiles
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.owner))
	for session := range p.owner {
		out = append(out, session)
	}
	return out
}
