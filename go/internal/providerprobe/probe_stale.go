package providerprobe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// R04S (#319 phase 2): what the product gateway does with a hook event or a confirmation that
// arrives late. No native runs: a producer posts to the product's own gateway (the one every
// probe session starts), and the checks read the gateway's backend, its stored profile
// snapshots and another live session's gateway.
//
// A "control" check is the same kind of event delivered while it is still current; it must be
// accepted, or the stale checks could pass only because nothing is ever accepted. A "stale"
// check is an event that arrives late (a left session, an earlier turn's nonce, another live
// session's link, a forged helper); each must give no execution right, no profile or route
// change, no backend request and no change to another session.

const (
	// ReasonStaleControlFailed is a control event that was not accepted, or the checks could
	// not observe: INCONCLUSIVE, never PASS.
	ReasonStaleControlFailed = "R04S_CONTROL_FAILED"
	// ReasonStaleEventAccepted is a stale event that gained a right or changed state: FAIL_SAFETY.
	ReasonStaleEventAccepted = "R04S_STALE_EVENT_ACCEPTED"
)

type p3StaleCheck struct {
	Name   string
	Kind   string // "control" or "stale"
	OK     bool
	Detail string `json:",omitempty"`
}

// p3StaleReport is R04S's verdict and the checks behind it.
type p3StaleReport struct {
	Verdict string
	Reasons []string
	Checks  []p3StaleCheck
}

// p3StaleGateway is the gateway under test as the checks see it.
type p3StaleGateway struct {
	URL, Token string
	// Projects is the transcript root a registration names; Events the native events directory
	// (confirmation phase only).
	Projects, Events string
	// Calls counts the backend requests the gateway made.
	Calls func() int
	// Snapshot is a digest of a session's stored profile, "" if it has none.
	Snapshot func(session string) string
}

// p3StaleSentinel is another live session's gateway, reached through a counting front.
type p3StaleSentinel struct {
	URL, Token string
	// Changed reports a request that reached it other than a confirmation, an outcome record, or
	// a routing mismatch; Confirmations counts the confirmations that did reach it.
	Changed       func() bool
	Confirmations func() int
}

// p3StaleMaker builds the gateways a run uses; the tests build stubs with a guard removed.
type p3StaleMaker struct {
	Gateway  func(t TB, confirmations bool) *p3StaleGateway
	Sentinel func(t TB) *p3StaleSentinel
}

// productStaleMaker is the product gateway, started by the same call every probe session uses.
var productStaleMaker = p3StaleMaker{
	Gateway: func(t TB, confirmations bool) *p3StaleGateway {
		transport := &p3Transport{world: func() *p3World { return nil }}
		g, err := app.StartProductGateway(transport)
		if err != nil {
			t.Fatal(err)
		}
		profiles, projects, events := t.TempDir(), t.TempDir(), t.TempDir()
		store, err := gateway.OpenSessionProfiles(profiles)
		if err != nil {
			g.Close(context.Background())
			t.Fatal(err)
		}
		g.ConfigureDelegations(projects)
		g.ConfigureSelection(bridge.Selection{}.WithCatalogue(p3Catalogue))
		g.ConfigureSessionProfiles(store, bridge.Pair{Model: "gpt-6-sol", Effort: "medium"}, nil, false, false)
		if confirmations {
			g.ConfigureNativeEvents(events)
			g.RequireNativeConfirmations()
		}
		t.Cleanup(func() {
			g.Close(context.Background())
			store.Close()
		})
		return &p3StaleGateway{URL: g.BaseURL(), Token: g.Token(), Projects: projects, Events: events,
			Calls: func() int { return len(transport.snapshot()) },
			Snapshot: func(session string) string {
				raw, err := os.ReadFile(filepath.Join(profiles, session+".json"))
				if err != nil {
					return ""
				}
				return digest(raw)
			}}
	},
	Sentinel: func(t TB) *p3StaleSentinel {
		g, err := app.StartProductGateway(&p3Transport{world: func() *p3World { return nil }})
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		records, paths := 0, map[string]int{}
		g.ObserveOutcome(func(gateway.RequestRecord) {
			mu.Lock()
			records++
			mu.Unlock()
		})
		target, _ := url.Parse(g.BaseURL())
		forward := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
		}}
		front := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths[r.URL.Path]++
			mu.Unlock()
			forward.ServeHTTP(rw, r)
		}))
		t.Cleanup(func() {
			front.Close()
			g.Close(context.Background())
		})
		return &p3StaleSentinel{URL: front.URL, Token: g.Token(),
			Changed: func() bool {
				mu.Lock()
				defer mu.Unlock()
				others := 0
				for path, n := range paths {
					if path != "/clauduct/confirmation" {
						others += n
					}
				}
				return records > 0 || others > 0 || g.RoutingMismatch()
			},
			Confirmations: func() int {
				mu.Lock()
				defer mu.Unlock()
				return paths["/clauduct/confirmation"]
			}}
	},
}

type staleRun struct {
	t      TB
	checks []p3StaleCheck
	client *http.Client
}

func (r *staleRun) check(kind, name string, ok bool, format string, args ...any) bool {
	detail := ""
	if format != "" {
		detail = fmt.Sprintf(format, args...)
	}
	r.checks = append(r.checks, p3StaleCheck{Name: name, Kind: kind, OK: ok, Detail: detail})
	return ok
}

// send posts a JSON body with the gateway's token and returns the status and the body's text.
func (r *staleRun) send(ctx context.Context, base, token, path string, headers map[string]string, body string) (int, string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return 0, err.Error()
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	return response.StatusCode, string(text)
}

const staleRequest = `{"model":"gpt-6-astra","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"ping"}]}`

func staleHeaders(session string) map[string]string {
	return map[string]string{"Anthropic-Version": "2023-06-01", "X-Claude-Code-Request-Class": "main", "X-Claude-Code-Session-Id": session}
}

// p3Stale runs R04S against the gateways maker builds.
func p3Stale(t TB, maker p3StaleMaker) p3StaleReport {
	t.Helper()
	r := &staleRun{t: t, client: &http.Client{Timeout: 10 * time.Second}}
	r.profilePhase(maker.Gateway(t, false))
	r.confirmationPhase(maker.Gateway(t, true), maker.Sentinel(t))
	report := p3StaleReport{Checks: r.checks, Verdict: providercert.Pass}
	for _, c := range r.checks {
		switch {
		case !c.OK && c.Kind == "control":
			report.Verdict = providercert.Inconclusive
			report.Reasons = append(report.Reasons, ReasonStaleControlFailed+":"+c.Name)
		}
	}
	if report.Verdict == providercert.Pass {
		for _, c := range r.checks {
			if !c.OK {
				report.Verdict = providercert.FailSafety
				report.Reasons = append(report.Reasons, ReasonStaleEventAccepted+":"+c.Name)
			}
		}
	}
	return report
}

func (r *staleRun) profilePhase(g *p3StaleGateway) {
	earlier, current := "30000000-0000-0000-0000-0000000000a1", "30000000-0000-0000-0000-0000000000a2"
	register := func(session, source string) int {
		fields := map[string]string{"event": "SessionStart", "sessionId": session, "transcriptPath": filepath.Join(g.Projects, session+".jsonl")}
		if source != "" {
			fields["source"] = source
		}
		body, _ := json.Marshal(fields)
		status, _ := r.send(context.Background(), g.URL, g.Token, "/clauduct/context", nil, string(body))
		return status
	}
	ask := func(session string) int {
		status, _ := r.send(context.Background(), g.URL, g.Token, "/v1/messages?beta=true", staleHeaders(session), staleRequest)
		return status
	}
	r.check("control", "register the earlier session", register(earlier, "startup") == http.StatusNoContent, "")
	r.check("control", "register the current session (the earlier one is left)", register(current, "clear") == http.StatusNoContent, "")
	served := ask(current)
	r.check("control", "the current session is served", served == http.StatusOK && g.Calls() == 1, "status=%d backend=%d", served, g.Calls())
	keptCurrent, keptEarlier := g.Snapshot(current), g.Snapshot(earlier)
	r.check("control", "the sessions have stored profiles", keptCurrent != "" && keptEarlier != "", "")

	// (i) The late UserPromptSubmit post is a SessionStart re-registration of the current session.
	late, lateClear := register(current, ""), register(current, "clear")
	r.check("control", "a late re-registration of the current session is accepted and idempotent",
		late == http.StatusNoContent && lateClear == http.StatusNoContent && g.Snapshot(current) == keptCurrent && g.Calls() == 1,
		"status=%d/%d unchanged=%t backend=%d", late, lateClear, g.Snapshot(current) == keptCurrent, g.Calls())

	// (ii) The same late post for a session this gateway has already left.
	leftStatus := register(earlier, "")
	r.check("stale", "a late registration of a left session is refused", leftStatus >= 400 && leftStatus < 500, "status=%d", leftStatus)
	r.check("stale", "a late registration of a left session changes no stored profile",
		g.Snapshot(earlier) == keptEarlier && g.Snapshot(current) == keptCurrent, "")
	leftAsk := ask(earlier)
	r.check("stale", "a left session reaches no backend after its late registration", leftAsk != http.StatusOK && g.Calls() == 1, "status=%d backend=%d", leftAsk, g.Calls())
	again := ask(current)
	r.check("control", "the current session is still served after the late events", again == http.StatusOK && g.Calls() == 2 && g.Snapshot(current) == keptCurrent,
		"status=%d backend=%d", again, g.Calls())
}

// staleChallenge is the confirmation request the gateway published for a waiting request.
type staleChallenge map[string]any

func (c staleChallenge) nonce() string { s, _ := c["nonce"].(string); return s }

func (c staleChallenge) reply(edit func(map[string]any)) string {
	out := map[string]any{}
	for k, v := range c {
		out[k] = v
	}
	out["confirmations"] = true
	if edit != nil {
		edit(out)
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func (r *staleRun) confirmationPhase(g *p3StaleGateway, sentinel *p3StaleSentinel) {
	session := "30000000-0000-0000-0000-0000000000b1"
	body, _ := json.Marshal(map[string]string{"event": "SessionStart", "sessionId": session, "source": "startup", "transcriptPath": filepath.Join(g.Projects, session+".jsonl")})
	status, text := r.send(context.Background(), g.URL, g.Token, "/clauduct/context", nil, string(body))
	if !r.check("control", "register the confirmation session", status == http.StatusNoContent, "status=%d %.80s", status, text) {
		return
	}
	kept := g.Snapshot(session)

	// publish is what native's plugin writes before a model request: the turn's receipt and step.
	publish := func(sequence int, turn string) {
		active := filepath.Join(g.Events, "active", "root")
		os.MkdirAll(active, 0o700)
		receipt, _ := json.Marshal(map[string]any{"session": session, "agent": "", "turn": turn})
		stem := fmt.Sprintf("%d-%s", sequence, turn)
		os.WriteFile(filepath.Join(active, stem+".json"), receipt, 0o600)
		os.WriteFile(filepath.Join(active, stem+".ready"), nil, 0o600)
		step, _ := json.Marshal(map[string]any{"session": session, "agent": "", "turn": turn, "index": 0, "eligible": false, "mode": "native_tui"})
		os.WriteFile(filepath.Join(g.Events, "step-root.json.tmp"), step, 0o600)
		os.Rename(filepath.Join(g.Events, "step-root.json.tmp"), filepath.Join(g.Events, "step-root.json"))
	}
	type answer struct {
		status int
		text   string
	}
	askAsync := func() (<-chan answer, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		out := make(chan answer, 1)
		go func() {
			s, text := r.send(ctx, g.URL, g.Token, "/v1/messages?beta=true", staleHeaders(session), staleRequest)
			out <- answer{s, text}
		}()
		return out, cancel
	}
	challengeAfter := func(previous string) (staleChallenge, bool) {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			raw, err := os.ReadFile(filepath.Join(g.Events, "confirmation-request.json"))
			var c staleChallenge
			if err == nil && json.Unmarshal(raw, &c) == nil && c.nonce() != "" && c.nonce() != previous {
				return c, true
			}
		}
		return nil, false
	}
	helper := func(base, token string, c staleChallenge, edit func(map[string]any)) (int, string) {
		return r.send(context.Background(), base, token, "/clauduct/confirmation", nil, c.reply(edit))
	}

	// Turn A: the current nonce's helper is the positive control.
	publish(1, "turnA")
	waitingA, cancelA := askAsync()
	defer cancelA()
	first, ok := challengeAfter("")
	if !r.check("control", "a model request publishes a confirmation challenge", ok, "") {
		return
	}
	connected := make(chan string, 1)
	helperDone := make(chan struct{})
	go func() {
		defer close(helperDone)
		request, _ := http.NewRequest(http.MethodPost, g.URL+"/clauduct/confirmation", strings.NewReader(first.reply(nil)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+g.Token)
		response, err := (&http.Client{}).Do(request) // held open until the request ends, as the real helper's is
		if err != nil {
			connected <- "error: " + err.Error()
			return
		}
		defer response.Body.Close()
		line, _ := bufio.NewReader(response.Body).ReadString('\n')
		connected <- fmt.Sprintf("%d %s", response.StatusCode, strings.TrimSpace(line))
		io.Copy(io.Discard, response.Body)
	}()
	var servedA answer
	select {
	case servedA = <-waitingA:
	case <-time.After(10 * time.Second):
		servedA = answer{status: -1}
	}
	r.check("control", "the current nonce's helper admits its request", servedA.status == http.StatusOK && g.Calls() == 1,
		"helper=%q status=%d backend=%d", func() string {
			select {
			case s := <-connected:
				return s
			default:
				return ""
			}
		}(), servedA.status, g.Calls())
	cancelA()
	<-helperDone

	// Turn A is over. Its nonce arrives late.
	lateStatus, _ := helper(g.URL, g.Token, first, nil)
	r.check("stale", "a late reply with an ended turn's nonce is stale", lateStatus == http.StatusGone, "status=%d", lateStatus)
	r.check("stale", "a late reply with an ended turn's nonce reaches no backend", g.Calls() == 1, "backend=%d", g.Calls())

	// Turn B waits for its own helper while every late and forged reply arrives.
	publish(2, "turnB")
	waitingB, cancelB := askAsync()
	defer cancelB()
	second, ok := challengeAfter(first.nonce())
	if !r.check("control", "the next turn publishes a new challenge", ok, "") {
		return
	}
	pending := func() bool {
		select {
		case <-waitingB:
			return false
		case <-time.After(300 * time.Millisecond):
			return true
		}
	}
	oldStatus, _ := helper(g.URL, g.Token, first, nil)
	r.check("stale", "the previous turn's nonce is refused while the next turn waits", oldStatus == http.StatusGone, "status=%d", oldStatus)
	foreign, _ := helper(sentinel.URL, sentinel.Token, second, nil)
	r.check("stale", "the waiting turn's reply to another live session's link is refused", foreign >= 400, "status=%d", foreign)
	r.check("stale", "another live session is not changed by that reply", !sentinel.Changed() && sentinel.Confirmations() == 1, "confirmations=%d changed=%t", sentinel.Confirmations(), sentinel.Changed())
	guessed, _ := helper(g.URL, g.Token, staleChallenge{"nonce": "forged-nonce-0000", "session": session, "agent": "", "turn": "turnB", "index": 0, "search": false}, nil)
	r.check("stale", "a forged helper with a guessed nonce is refused", guessed >= 400, "status=%d", guessed)
	for name, edit := range map[string]func(map[string]any){
		"another session":       func(m map[string]any) { m["session"] = "30000000-0000-0000-0000-0000000000b2" },
		"an earlier turn":       func(m map[string]any) { m["turn"] = "turnA" },
		"another step":          func(m map[string]any) { m["index"] = 7 },
		"no confirmation":       func(m map[string]any) { m["confirmations"] = false },
		"a different agent":     func(m map[string]any) { m["agent"] = "forged-agent" },
		"a different class":     func(m map[string]any) { m["auxiliary"] = true },
		"a different tool path": func(m map[string]any) { m["search"] = true },
	} {
		status, _ := helper(g.URL, g.Token, second, edit)
		r.check("stale", "a forged helper reply naming "+name+" is refused", status >= 400, "status=%d", status)
	}
	// A helper that announces the connection without making one posts nothing at all.
	r.check("stale", "no stale or forged reply admitted the waiting request", pending() && g.Calls() == 1 && g.Snapshot(session) == kept,
		"backend=%d profileKept=%t", g.Calls(), g.Snapshot(session) == kept)
	cancelB()
	// The gateway notices the cancelled request a moment later; until it has, the nonce is still
	// pending (and its lease already cancelled, so it admits nothing). Stale is where it settles.
	endedStatus := 0
	for attempt := 0; attempt < 10 && endedStatus != http.StatusGone; attempt++ {
		time.Sleep(100 * time.Millisecond)
		endedStatus, _ = helper(g.URL, g.Token, second, nil)
	}
	r.check("stale", "a reply after its request ended is stale", endedStatus == http.StatusGone, "status=%d", endedStatus)
	r.check("stale", "the late replies reached the backend zero times", g.Calls() == 1 && g.Snapshot(session) == kept, "backend=%d", g.Calls())
}
