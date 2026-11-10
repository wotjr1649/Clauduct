package bridge

import (
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func fixtureModel(key string) Model {
	for _, model := range LegacyModels() {
		if model.Key == key {
			return model
		}
	}
	panic("missing synthetic catalogue model: " + key)
}

func requireUnknownModels(t testing.TB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, offered := testCatalogue.ByID(id); offered {
			t.Fatalf("unknown fixture model %q is offered", id)
		}
		if _, retired := Retired[id]; retired {
			t.Fatalf("unknown fixture model %q is retired", id)
		}
	}
}

// A synthetic account list standing in for the one a session fetches at start. Its default
// levels are the v0.6.3 factory efforts, so tests written against those keep their meaning.
var testCatalogue = func() *Catalogue {
	all := []string{"low", "medium", "high", "xhigh", "max"}
	var models []AccountModel
	for _, item := range []struct{ key, effort string }{
		{"astra", "medium"}, {"sol", "xhigh"}, {"sol6", "xhigh"}, {"terra", "high"}, {"luna", "max"},
	} {
		models = append(models, AccountModel{ID: fixtureModel(item.key).ID, Efforts: all, Default: item.effort, Visible: true})
	}
	c, _, err := NewCatalogue(models)
	if err != nil {
		panic(err)
	}
	return c
}()

var testSelection = Selection{}.WithCatalogue(testCatalogue)

var Models = testCatalogue.Models()

var Efforts = testCatalogue.Efforts()

func SelectRoute(requested, effort string) (Route, error) {
	return testSelection.SelectRoute(requested, effort)
}

func RoleRoute(role string) (Route, bool) { return testSelection.RoleRoute(role) }

func ModelByID(id string) (Model, bool) { return testSelection.ModelByID(id) }

func BuildRequest(request *anthropic.Request, override ...Route) (*Request, error) {
	return testSelection.BuildRequest(request, override...)
}

func ResolveRoute(request *anthropic.Request, override ...Route) (Route, error) {
	return testSelection.ResolveRoute(request, override...)
}

func catalogueRoutes() []Route {
	var out []Route
	for _, model := range Models {
		out = append(out, Route{Model: model.ID, Effort: model.Effort, Source: "catalogue"})
	}
	return out
}
