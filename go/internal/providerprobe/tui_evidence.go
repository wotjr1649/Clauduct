package providerprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// p3TUISubmitWindow is how long the driver waits, after typing a prompt, for the receiver to
// see that turn's canary. It is a window for the submission to show, not a response wait: a
// local request reaches a receiver within a second or two once Enter was taken.
const p3TUISubmitWindow = 10 * time.Second

// p3TUIResendEnter is whether the driver sends Enter once more when the window passes with no
// request. Measured 2026-10-08 (native 2.1.293): the prompt sometimes sits in the input box with
// no request (3 of 24 prompts in 6 runs); a resend 10 seconds later recovered none of the 3, so it
// is off: it only delayed the same failure. A resend, if turned on, is a driver mark and is shown
// in the case's evidence and errors (ENTER_RESENT), never silent.
const p3TUIResendEnter = false

// p3SubmitStep names a driver step that types a prompt and the turn whose canary its request
// carries.
type p3SubmitStep struct {
	Index int
	Turn  string
}

// addSubmitChecks makes each named step of the driver's script check its submission against a
// file the probe writes when that turn's canary is seen (submitFile), within window.
func addSubmitChecks(stepsFile string, submits []p3SubmitStep, file func(turn string) string, window time.Duration, resend bool) error {
	raw, err := os.ReadFile(stepsFile)
	if err != nil {
		return err
	}
	var steps []map[string]any
	if err := json.Unmarshal(raw, &steps); err != nil {
		return err
	}
	for _, sub := range submits {
		if sub.Index < 0 || sub.Index >= len(steps) {
			return fmt.Errorf("no driver step %d to check", sub.Index)
		}
		steps[sub.Index]["confirm_file"] = file(sub.Turn)
		steps[sub.Index]["confirm_ms"] = window.Milliseconds()
		steps[sub.Index]["confirm_resend"] = resend
	}
	raw, err = json.Marshal(steps)
	if err != nil {
		return err
	}
	return os.WriteFile(stepsFile, raw, 0o600)
}

// p3SubmitFile is where the probe tells the driver turn's canary was seen.
func p3SubmitFile(dir, turn string) string { return filepath.Join(dir, "submitted-"+turn) }

// watchSubmits writes each turn's submit file as soon as a receiver saw that turn's canary, and
// records when. It ends when stop closes.
func watchSubmits(w *p3World, dir string, turns []string, stop <-chan struct{}) {
	pending := slices.Clone(turns)
	for len(pending) > 0 {
		for i := 0; i < len(pending); i++ {
			if p3CanarySeenAnywhere(w, pending[i]) {
				os.WriteFile(p3SubmitFile(dir, pending[i]), []byte("ready\n"), 0o600)
				pending = slices.Delete(pending, i, i+1)
				i--
			}
		}
		select {
		case <-stop:
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// p3CanaryFirstSeen is when a receiver or the gateway first saw canary id.
func p3CanaryFirstSeen(w *p3World, id string) (time.Time, bool) {
	var first time.Time
	note := func(at time.Time) {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	for _, call := range w.ingressWithOutcomes() {
		if slices.Contains(call.canaries, id) {
			note(call.at)
		}
	}
	for _, r := range w.allReceivers() {
		seen, _, _ := r.Observer.Seen()
		for _, s := range seen {
			if slices.Contains(s.Canaries, id) {
				note(s.At)
			}
		}
	}
	return first, !first.IsZero()
}

// p3FirstRequest is when anything first reached the gateway or a receiver.
func p3FirstRequest(w *p3World) (time.Time, bool) {
	var first time.Time
	note := func(at time.Time) {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	for _, call := range w.ingressWithOutcomes() {
		note(call.at)
	}
	for _, r := range w.allReceivers() {
		seen, _, _ := r.Observer.Seen()
		for _, s := range seen {
			note(s.At)
		}
	}
	return first, !first.IsZero()
}

// p3TUIPromptTiming is one typed prompt's timeline.
type p3TUIPromptTiming struct {
	Turn string
	// TypedAt is when the driver began the prompt's input; CanaryAt when a receiver or the gateway
	// first saw that turn's canary ("" when none did).
	TypedAt, CanaryAt string
	TypedToCanaryMs   int64
	// Submit is the driver's own submission check line (confirmed, by what, how long, resent).
	Submit string
}

// p3TUIEvidence is what a TUI run leaves besides the screen: the driver's stdout and marks, the
// end of its raw log, and the timeline from native's start to each prompt's request. It is kept
// for every TUI run, passed or not. Canary values are replaced by their IDs.
type p3TUIEvidence struct {
	DriverStdout string `json:",omitempty"`
	DriverStderr string `json:",omitempty"`
	Marks        string `json:",omitempty"`
	RawTail      string `json:",omitempty"`
	// StepsDeadlineS is the driver's -deadline: its steps ended before the case's limit.
	StepsDeadlineS int
	SpawnedAt      string `json:",omitempty"`
	FirstRequestAt string `json:",omitempty"`
	// SpawnToFirstRequestMs is native's start (pinned image confirmed) to the first request.
	SpawnToFirstRequestMs int64
	Prompts               []p3TUIPromptTiming
	// Resent is the number of Enter resends the driver made; each is also a mark.
	Resent int
	// SubmitUnconfirmed lists the prompts the driver typed that no receiver saw (SUBMIT_UNCONFIRMED).
	SubmitUnconfirmed []string `json:",omitempty"`
	// SessionBefore and SessionAfter are the product's session record when the stimulus began and
	// at the end (profile snapshot digests by session ID, transcript IDs).
	SessionBefore, SessionAfter *p3SessionFacts `json:",omitempty"`
	// EnterResent lists the prompts whose submission needed the one Enter resend.
	EnterResent []string `json:",omitempty"`
}

// tuiMarkLines reads the marks file as lines.
func tuiMarkLines(marks string) []string {
	raw, err := os.ReadFile(marks)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// tuiSubmitLine is the driver's mark for the submission check at step index.
func tuiSubmitLine(lines []string, index int) (detail string, ok bool) {
	prefix := "submit" + strconv.Itoa(index) + " "
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", false
}

// tuiResends counts the driver's Enter resend marks.
func tuiResends(lines []string) int {
	n := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "resend") {
			n++
		}
	}
	return n
}

// p3SubmitUnconfirmed is the SUBMIT_UNCONFIRMED record for a prompt the driver typed that the
// receivers never saw, with the timings that locate the loss.
func p3SubmitUnconfirmed(prompt p3TUIPromptTiming, spawnedAt time.Time, typed time.Time) string {
	since := "unknown"
	if !spawnedAt.IsZero() && !typed.IsZero() {
		since = typed.Sub(spawnedAt).Round(time.Millisecond).String()
	}
	return fmt.Sprintf("SUBMIT_UNCONFIRMED %s: typed at %s (%s after native start), no request carried its canary; driver check: %s",
		prompt.Turn, prompt.TypedAt, since, prompt.Submit)
}

// allReceivers are the world's wrong destinations a TUI run can reach.
func (w *p3World) allReceivers() []*providercert.Receiver {
	return []*providercert.Receiver{w.sgA, w.sgB, w.settings, w.socket}
}

// p3TUIEvidenceOf gathers a TUI run's driver evidence after the run: the driver's stdout and
// stderr, its marks, the end of its raw log, and when each prompt was typed and first seen.
func p3TUIEvidenceOf(w *p3World, obs *p3Observed, marks, raw, stdout, stderr string, submits []p3SubmitStep, spawnedAt time.Time, deadlineS int) *p3TUIEvidence {
	redact := func(text string) string { return p3RedactCanaries(text, obs.Turns) }
	lines := tuiMarkLines(marks)
	e := &p3TUIEvidence{SessionBefore: obs.SessionBefore, SessionAfter: obs.SessionAfter, DriverStdout: redact(stdout), DriverStderr: redact(stderr), Marks: redact(strings.Join(lines, "\n")),
		RawTail: redact(rawScreen(raw)), StepsDeadlineS: deadlineS, Resent: tuiResends(lines)}
	if !spawnedAt.IsZero() {
		e.SpawnedAt = spawnedAt.Format(time.RFC3339Nano)
	}
	if first, ok := p3FirstRequest(w); ok {
		e.FirstRequestAt = first.Format(time.RFC3339Nano)
		if !spawnedAt.IsZero() {
			e.SpawnToFirstRequestMs = first.Sub(spawnedAt).Milliseconds()
		}
	}
	for _, sub := range submits {
		timing := p3TUIPromptTiming{Turn: sub.Turn}
		typed, typedOK := markAt(marks, sub.Index)
		if typedOK {
			timing.TypedAt = typed.Format(time.RFC3339Nano)
		}
		seenAt, seen := p3CanaryFirstSeen(w, sub.Turn)
		if seen {
			timing.CanaryAt = seenAt.Format(time.RFC3339Nano)
			if typedOK {
				timing.TypedToCanaryMs = seenAt.Sub(typed).Milliseconds()
			}
		}
		timing.Submit, _ = tuiSubmitLine(lines, sub.Index)
		if strings.Contains(timing.Submit, "resent=true") {
			e.EnterResent = append(e.EnterResent, fmt.Sprintf("ENTER_RESENT %s: no request carried its canary within %s of typing; the driver sent Enter once more; driver check: %s", sub.Turn, p3TUISubmitWindow, timing.Submit))
		}
		e.Prompts = append(e.Prompts, timing)
		if typedOK && !seen {
			e.SubmitUnconfirmed = append(e.SubmitUnconfirmed, p3SubmitUnconfirmed(timing, spawnedAt, typed))
		}
	}
	return e
}

// stopped reports whether stop is closed.
func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// p3ScreenOrdered reports whether screen, whitespace removed, shows each of order's texts in
// order. An empty order is trivially shown.
func p3ScreenOrdered(screen string, order []string) bool {
	rest := strings.Join(strings.Fields(screen), "")
	for _, text := range order {
		i := strings.Index(rest, text)
		if i < 0 {
			return false
		}
		rest = rest[i+len(text):]
	}
	return true
}

// p3SessionFacts is the product's record of the sessions a world has: each profile snapshot's
// digest by session ID, and the IDs native wrote transcripts for.
type p3SessionFacts struct {
	Profiles    map[string]string
	Transcripts []string
}

// sessionFacts reads the world's session record.
func (w *p3World) sessionFacts() *p3SessionFacts {
	f := &p3SessionFacts{Profiles: map[string]string{}}
	if entries, err := os.ReadDir(filepath.Join(w.home, ".clauduct", "sessions")); err == nil {
		for _, e := range entries {
			id, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok || e.IsDir() {
				continue
			}
			if raw, err := os.ReadFile(filepath.Join(w.home, ".clauduct", "sessions", e.Name())); err == nil {
				f.Profiles[id] = p3ProfileDigest(raw)
			}
		}
	}
	filepath.Walk(filepath.Join(w.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if id, ok := strings.CutSuffix(filepath.Base(path), ".jsonl"); ok {
				f.Transcripts = append(f.Transcripts, id)
			}
		}
		return nil
	})
	slices.Sort(f.Transcripts)
	return f
}

// same reports whether the later record has the same sessions with the same profile snapshots.
func (f *p3SessionFacts) same(later *p3SessionFacts) bool {
	return f != nil && later != nil && maps.Equal(f.Profiles, later.Profiles) && slices.Equal(f.Transcripts, later.Transcripts)
}

// p3ModelSelections are the model choices a set of calls carried: what native asked the gateway
// for, and the model and effort the backend would run.
type p3ModelSelections map[string]bool

// p3SelectionsOf collects the selections of the calls that carried canary id and, when without
// is set, not that canary.
func p3SelectionsOf(obs *p3Observed, id, without string) (ingress, backend p3ModelSelections) {
	ingress, backend = p3ModelSelections{}, p3ModelSelections{}
	pick := func(calls []p3Call, into p3ModelSelections, key func(p3Call) string) {
		for _, c := range calls {
			if slices.Contains(c.canaries, id) && (without == "" || !slices.Contains(c.canaries, without)) {
				into[key(c)] = true
			}
		}
	}
	pick(obs.Ingress, ingress, func(c p3Call) string { return c.model })
	pick(obs.Calls, backend, func(c p3Call) string { return c.backendModel + "/" + c.backendEffort })
	return ingress, backend
}

// within reports whether every selection in s is also in of, and s is not empty.
func (s p3ModelSelections) within(of p3ModelSelections) bool {
	if len(s) == 0 {
		return false
	}
	for k := range s {
		if !of[k] {
			return false
		}
	}
	return true
}

// p3ProfileDigest is the digest of a session record's profile: its ID, the model selection, the
// last pair and the parent. The record's transcript offset and tool-call table advance with every
// turn and are not the profile.
func p3ProfileDigest(raw []byte) string {
	var record map[string]json.RawMessage
	if json.Unmarshal(raw, &record) != nil {
		sum := sha256.Sum256(raw)
		return "unparsed:" + hex.EncodeToString(sum[:])
	}
	profile := map[string]json.RawMessage{}
	for _, key := range []string{"id", "selection", "last", "parent"} {
		if v, ok := record[key]; ok {
			profile[key] = v
		}
	}
	canonical, _ := json.Marshal(profile)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
