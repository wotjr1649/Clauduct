package bridge

import "github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"

// A synthetic account list standing in for the one a session fetches at start. Its default
// levels are the v0.6.3 factory efforts, so tests written against those keep their meaning.
var testCatalogue = func() *Catalogue {
	all := []string{"low", "medium", "high", "xhigh", "max"}
	c, _, err := NewCatalogue([]AccountModel{
		{ID: "gpt-6-astra", Efforts: all, Default: "medium", Visible: true},
		{ID: "gpt-6.1-sol", Efforts: all, Default: "xhigh", Visible: true},
		{ID: "gpt-6-sol", Efforts: all, Default: "xhigh", Visible: true},
		{ID: "gpt-5.6-terra", Efforts: all, Default: "high", Visible: true},
		{ID: "gpt-6-luna", Efforts: all, Default: "max", Visible: true},
	})
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
