package bridge

import (
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
)

// CAP01. Which model a request runs on decides what the user is billed, so these values
// came from src/models.mjs and src/agent-selection.mjs:23-27 rather than from a convention
// that looked reasonable.
//
// One of them no longer matches that source and the divergence is deliberate: sonnet routes
// to terra here, where the baseline sends it to luna. The baseline's table put sonnet and
// haiku on the same model and left terra unreachable by any Claude name, so two picker
// entries ran the identical route and a fourth model could not be selected at all. The four
// tiers now map onto the four models.
func TestClaudeNamesRouteToTheTiersTheyBelongTo(t *testing.T) {
	for requested, want := range map[string]Route{
		// Short aliases.
		"haiku":  {Model: "gpt-5.6-luna", Effort: "max", Source: "alias"},
		"sonnet": {Model: "gpt-5.6-terra", Effort: "high", Source: "alias"},
		"opus":   {Model: "gpt-5.6-sol", Effort: "xhigh", Source: "alias"},
		"fable":  {Model: "gpt-6-astra", Effort: "medium", Source: "alias"},

		// Versioned ids, matched by family so no version is pinned here.
		"claude-opus-5":              {Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"},
		"claude-opus-4-1":            {Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"},
		"claude-sonnet-5":            {Model: "gpt-5.6-terra", Effort: "high", Source: "family"},
		"claude-haiku-4-5-20251001":  {Model: "gpt-5.6-luna", Effort: "max", Source: "family"},
		"claude-fable-5-1":           {Model: "gpt-6-astra", Effort: "medium", Source: "family"},
		"claude-opus-99-not-yet-out": {Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"},

		// Catalogue keys and backend ids named outright.
		"luna":          {Model: "gpt-5.6-luna", Effort: "max", Source: "catalogue"},
		"terra":         {Model: "gpt-5.6-terra", Effort: "high", Source: "catalogue"},
		"gpt-6-astra":   {Model: "gpt-6-astra", Effort: "medium", Source: "direct"},
		"gpt-5.6-terra": {Model: "gpt-5.6-terra", Effort: "high", Source: "direct"},
	} {
		t.Run(requested, func(t *testing.T) {
			got, err := SelectRoute(requested, "")
			if err != nil {
				t.Fatalf("SelectRoute(%q): %v", requested, err)
			}
			if got != want {
				t.Fatalf("SelectRoute(%q) = %+v, want %+v", requested, got, want)
			}
		})
	}
}

// CAP02: an unsupported model or effort is refused, never quietly replaced.
//
// A default would run the request on a model the user did not ask for and bill them for it.
// The failure a user can see and fix is better than the one they find on an invoice.
func TestAnUnsupportedRouteIsRefusedNotDefaulted(t *testing.T) {
	for name, tc := range map[string]struct{ model, effort string }{
		"a model nobody offers":       {"gpt-9-nonesuch", ""},
		"an empty model":              {"", ""},
		"a bare family name":          {"claude-opus", ""},
		"a family with no suffix":     {"claude-sonnet", ""},
		"a near miss on an alias":     {"sonet", ""},
		"a near miss on a backend id": {"gpt-5.6-lunaa", ""},
		"case does not fold":          {"Sonnet", ""},
		"a prefix of a backend id":    {"gpt-5.6", ""},
		"an effort nobody offers":     {"sonnet", "ultra"},
		"an effort with wrong case":   {"sonnet", "Low"},
		"an effort that is a number":  {"sonnet", "5"},
		"whitespace around a model":   {" sonnet", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := SelectRoute(tc.model, tc.effort)
			if !errors.Is(err, ErrUnsupportedRoute) {
				t.Fatalf("SelectRoute(%q, %q) = %+v, %v; want a refusal",
					tc.model, tc.effort, got, err)
			}
			if got.Model != "" {
				t.Fatalf("a refused route still named %q", got.Model)
			}
		})
	}
}

// An explicit effort replaces the model's default, and the record says so.
func TestAnExplicitEffortIsRecordedAsSuch(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		got, err := SelectRoute("sonnet", effort)
		if err != nil {
			t.Fatalf("SelectRoute(sonnet, %q): %v", effort, err)
		}
		if got.Effort != effort {
			t.Fatalf("effort = %q, want %q", got.Effort, effort)
		}
		if !strings.HasSuffix(got.Source, "+effort") {
			t.Fatalf("source = %q; a caller who sees a surprising effort needs to know it "+
				"was theirs", got.Source)
		}
	}
}

// The efforts are not uniform across models and must not be normalised. Making them the
// same would change what every request costs.
func TestEachModelKeepsItsOwnDefaultEffort(t *testing.T) {
	seen := map[string]string{}
	for _, model := range Models {
		seen[model.Effort] = model.Key
	}
	if len(seen) < 3 {
		t.Fatalf("the catalogue has collapsed to %d distinct efforts: %v. The baseline "+
			"gives each model its own, and levelling them changes what a request costs.",
			len(seen), seen)
	}
}

// The reply names the model that answered, which is the route and not the request.
//
// This test said the opposite until 2026-09-18, and said it with a reason: "the client is
// told it talked to something it never requested". The reason has the honesty backwards. A
// role-routed subagent does not talk to what it requested, so naming the request is the
// statement that is false, and it is false in the direction that hides a model swap from the
// user -- which ARCHITECTURE.md:217 says this build does not do.
//
// The Node baseline had it this way from the start: frameStart uses prepared.selected.model
// (src/native-protocol.mjs:509-511), selected comes from the role route when there is one
// (:258), and its own tests pin it -- a client posting terra gets ROLE_MODELS.Plan.model back
// in the stream (src/test-native.mjs:505-511). The rewrite diverged here by omission, and the
// divergence was found by running a real session, not by reading either one.
//
// Reversing a recorded decision, and recorded as such: the old invariant and its reason are
// above, the measurement that overturned it is in PARITY.md.
func TestTheReplyNamesTheModelThatAnswered(t *testing.T) {
	request := decodeRequest(t, `{"model":"claude-opus-5","max_tokens":100000,"stream":true,`+
		`"messages":[{"role":"user","content":"x"}]}`)

	backend, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if backend.Model != "gpt-5.6-sol" {
		t.Fatalf("the backend request named %q", backend.Model)
	}

	// The gateway states the route it built; runFor's empty value is the caller that has
	// none, and both are checked because only one of them is the product path.
	for name, tc := range map[string]struct{ effective, want string }{
		"the gateway, which knows the route": {backend.Model, "gpt-5.6-sol"},
		"a caller with no route to state":    {"", "claude-opus-5"},
	} {
		t.Run(name, func(t *testing.T) {
			frames, err := runForOn(t, request, tc.effective,
				itemAdded(0, `{"id":"msg_1","type":"message"}`),
				textDelta(0, "hi"), textDone(0, "hi"),
				itemDone(0, messageItem("msg_1", "hi")),
				event(codex.Completed, completedOK))
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}
			start := blockAt(t, frames, "message_start", 0)
			message := start["message"].(map[string]any)
			if message["model"] != tc.want {
				t.Fatalf("the client was told it talked to %v, want %s", message["model"], tc.want)
			}
		})
	}
}

// Every backend model must be reachable by a Claude name.
//
// This is the invariant whose violation nobody noticed. The catalogue had four models and
// the alias table had four Claude tiers, but sonnet and haiku both pointed at luna, so
// terra had no name the client could send and the user's picker could not reach it. One
// table now holds both halves, and this fails if a model is ever added without one.
func TestEveryModelIsReachableByAClaudeName(t *testing.T) {
	for _, model := range Models {
		if model.Alias == "" || model.Family == "" {
			t.Errorf("%s has no Claude name, so the client cannot ask for it", model.ID)
			continue
		}
		for _, requested := range []string{model.Key, model.Alias, model.Family + "5", model.ID} {
			route, err := SelectRoute(requested, "")
			if err != nil {
				t.Errorf("SelectRoute(%q): %v", requested, err)
				continue
			}
			if route.Model != model.ID {
				t.Errorf("SelectRoute(%q) = %s, want %s", requested, route.Model, model.ID)
			}
		}
	}
	// And no two models may share a name, which is the other half of the same mistake.
	for _, field := range []func(Model) string{
		func(m Model) string { return m.Key },
		func(m Model) string { return m.ID },
		func(m Model) string { return m.Alias },
		func(m Model) string { return m.Family },
	} {
		seen := map[string]string{}
		for _, model := range Models {
			name := field(model)
			if other, taken := seen[name]; taken {
				t.Errorf("%s and %s both answer to %q, so one of them is unreachable",
					other, model.ID, name)
			}
			seen[name] = model.ID
		}
	}
}

// A name that looks like a menu entry and is not one resolves to nothing.
//
// Nothing resolves to a guess. A subagent whose type this build does not recognise keeps the
// model the client chose, which is the same answer every other routing failure gives.
func TestANameThatIsNotAMenuEntryRoutesNowhere(t *testing.T) {
	for _, role := range []string{
		"clauduct-inherit",       // deliberate: the child keeps the parent's route
		"clauduct-terra",         // no effort
		"clauduct-terra-",        // an empty one
		"clauduct-terra-extreme", // not an effort this backend has
		"clauduct-venus-high",    // not a model in the catalogue
		"clauduct-",
		"clauduct",
		"terra-high",              // the prefix is what says this build defined it
		"CLAUDUCT-TERRA-HIGH",     // and it is not case-insensitive
		"gpt-5.6-terra",           // a model name is not an agent type
		"some-users-own-agent",    // theirs, and theirs to route
		"clauduct-terra-high-ish", // effort is the rest of the name, not a prefix of it
	} {
		if route, known := RoleRoute(role); known {
			t.Errorf("%q resolved to %s/%s", role, route.Model, route.Effort)
		}
	}
}

// And one that is resolves to exactly what its name says.
func TestAMenuEntryResolvesToWhatItsNameSays(t *testing.T) {
	for role, want := range map[string]Route{
		"clauduct-terra-high":  {Model: "gpt-5.6-terra", Effort: "high", Source: "role"},
		"clauduct-astra-low":   {Model: "gpt-6-astra", Effort: "low", Source: "role"},
		"clauduct-luna-max":    {Model: "gpt-5.6-luna", Effort: "max", Source: "role"},
		"clauduct-sol-xhigh":   {Model: "gpt-5.6-sol", Effort: "xhigh", Source: "role"},
		"clauduct-terra-low":   {Model: "gpt-5.6-terra", Effort: "low", Source: "role"},
		"clauduct-astra-xhigh": {Model: "gpt-6-astra", Effort: "xhigh", Source: "role"},
	} {
		route, known := RoleRoute(role)
		if !known || route != want {
			t.Errorf("%q = %+v (%v), want %+v", role, route, known, want)
		}
	}
}

// A role that inherits is known, and is not the same answer as a role nobody has looked at.
func TestAnInheritingRoleIsKnownWithoutHavingARoute(t *testing.T) {
	if !InheritsParent("workflow-subagent") {
		t.Error("the role every Workflow agent reports is not recognised")
	}
	if route, known := RoleRoute("workflow-subagent"); known {
		t.Errorf("it was reassigned to %s/%s; inheriting means not reassigning",
			route.Model, route.Effort)
	}
	for _, role := range []string{"Explore", "Plan", "general-purpose",
		"clauduct-terra-high", "some-users-own-agent", ""} {
		if InheritsParent(role) {
			t.Errorf("%q was treated as a deliberate inherit", role)
		}
	}
}

// The effort a system turn asks for is the effort the turn runs at.
//
// Found by review: the decoder read output_config.effort off a system turn and nothing
// ever looked at it, so a turn asking for high ran at whatever the session was using.
// Accepted and ignored, which is the failure this package's doc names first -- and the
// same shape as the schema that was being validated and dropped.
func TestASystemTurnSetsTheEffortForThatTurn(t *testing.T) {
	const head = `{"model":"sonnet","max_tokens":16,"stream":true,"messages":[` +
		`{"role":"user","content":"x"},`

	plain, err := BuildRequest(decodeRequest(t, head+`{"role":"assistant","content":"y"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	asked, err := BuildRequest(decodeRequest(t,
		head+`{"role":"system","content":"go carefully","output_config":{"effort":"xhigh"}}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if asked.Effort.Effort != "xhigh" {
		t.Fatalf("effort = %q, want the xhigh the turn asked for (plain turn: %q)",
			asked.Effort.Effort, plain.Effort.Effort)
	}
	// The record says which rule produced it, which is what CAP03 is for: a reader seeing
	// an effort the session never chose needs to know where it came from.
	if !strings.HasSuffix(asked.Source, "+turn") {
		t.Fatalf("source = %q, want it to name the turn", asked.Source)
	}
	// The model is untouched. A turn sets how hard, not what runs.
	if asked.Model != plain.Model {
		t.Fatalf("model moved from %q to %q", plain.Model, asked.Model)
	}

	// A role override wins: a subagent routed by what it is doing does not take an effort
	// out of the transcript. The baseline draws the same line.
	role := Route{Model: "gpt-5.6-luna", Effort: "max", Source: "role"}
	routed, err := BuildRequest(decodeRequest(t,
		head+`{"role":"system","content":"go carefully","output_config":{"effort":"low"}}]}`), role)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if routed.Effort.Effort != "max" {
		t.Fatalf("a role route was overwritten by a turn: %q", routed.Effort.Effort)
	}

	// An effort nobody defines is refused rather than forwarded.
	if _, err := BuildRequest(decodeRequest(t,
		head+`{"role":"system","content":"x","output_config":{"effort":"maximum"}}]}`)); err == nil {
		t.Fatal("an undefined effort was accepted")
	}
}

// Session 36, measured in a real session on 2026-09-18. The launcher ships an agent type
// the router does not know about, so using it reports the session as faulty.
//
// clauduct-inherit is one of the fourteen entries in the delegation menu (app/agents.go) and
// its whole point is to keep the parent's model and effort. The router had no entry for it:
// menuRoute rejects it because "inherit" carries no -<effort> suffix, inheritRoles held only
// workflow-subagent, so it fell through to the routing-miss counter. A session that used it
// came back with agents.unrouted=1 and the whole account dumped at exit.
//
// That is the failure inheritRoles was written to prevent, stated in its own comment for
// workflow-subagent: a diagnostic that cries wolf on ordinary use stops being read. This
// build's own agent was the one it cried wolf about.
func TestTheInheritAgentIsNotARoutingMiss(t *testing.T) {
	if !InheritsParent(InheritRole) {
		t.Errorf("%s is in the delegation menu and the router counts it as unrouted", InheritRole)
	}
	// Deliberately not a route: inheriting means keeping what the client asked for, so
	// RoleRoute must decline rather than reassign.
	if route, known := RoleRoute(InheritRole); known {
		t.Errorf("%s was reassigned to %s/%s; inheriting means not choosing",
			InheritRole, route.Model, route.Effort)
	}
	// And the name the menu builds is the name the router matches. Two spellings of one
	// string is how this broke.
	if InheritRole != MenuPrefix+"inherit" {
		t.Errorf("InheritRole = %q, which is not a %s name", InheritRole, MenuPrefix)
	}
}
