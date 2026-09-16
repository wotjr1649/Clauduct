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

// The route is what goes upstream. The reply must still name the model the client asked
// for, or the client is told it talked to something it never requested.
func TestTheReplyNamesTheRequestedModelNotTheRoutedOne(t *testing.T) {
	request := decodeRequest(t, `{"model":"claude-opus-5","max_tokens":100000,"stream":true,`+
		`"messages":[{"role":"user","content":"x"}]}`)

	backend, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if backend.Model != "gpt-5.6-sol" {
		t.Fatalf("the backend request named %q", backend.Model)
	}

	frames, err := runFor(t, request,
		itemAdded(0, `{"id":"msg_1","type":"message"}`),
		textDelta(0, "hi"), textDone(0, "hi"),
		itemDone(0, messageItem("msg_1", "hi")),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	start := blockAt(t, frames, "message_start", 0)
	message := start["message"].(map[string]any)
	if message["model"] != "claude-opus-5" {
		t.Fatalf("the client was told it talked to %v, not what it asked for", message["model"])
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
