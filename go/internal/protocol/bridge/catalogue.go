package bridge

import (
	"errors"
	"fmt"
	"slices"
)

// AccountModel is one entry of the Codex account's model list, already decoded by the
// caller. Only the fields routing needs are carried; descriptions and instructions never
// reach this package.
type AccountModel struct {
	ID      string
	Efforts []string
	Default string
	Visible bool
}

// Catalogue is one session's immutable account model list. A session fixes it at start
// and hands the same value to display, routing, Agent schemas and native hooks; a newer
// list applies from the next session. A nil Catalogue offers no model.
type Catalogue struct {
	models []Model
}

// ErrNoAccountModels means the account list offered no model this build can route.
var ErrNoAccountModels = errors.New("ACCOUNT_MODEL_LIST_UNAVAILABLE")

// NewCatalogue checks an account list and joins it with the legacy identities. Models
// whose levels native cannot express at all are left out and reported in skipped; every
// other model is offered as the account lists it.
func NewCatalogue(models []AccountModel) (catalogue *Catalogue, skipped []string, err error) {
	seen := map[string]bool{}
	c := &Catalogue{}
	for _, entry := range models {
		if !ValidModelID(entry.ID) || seen[entry.ID] {
			return nil, nil, fmt.Errorf("%w: model %q", ErrNoAccountModels, entry.ID)
		}
		seen[entry.ID] = true
		model, known := legacyByID(entry.ID)
		if !known {
			model = Model{ID: entry.ID, Menu: entry.ID}
		}
		model.Visible = entry.Visible
		for _, effort := range lowToMax {
			if slices.Contains(entry.Efforts, effort) {
				model.Efforts = append(model.Efforts, effort)
			}
		}
		if len(model.Efforts) == 0 {
			skipped = append(skipped, entry.ID)
			continue
		}
		if slices.Contains(model.Efforts, entry.Default) {
			model.Effort = entry.Default
		}
		c.models = append(c.models, model)
	}
	if len(c.models) == 0 {
		return nil, skipped, ErrNoAccountModels
	}
	// A new model's menu name is its ID. One that would read as another entry -- a legacy
	// key, the inherit entry or an old per-effort name -- gets no menu entry and stays
	// selectable by its ID through the Agent model argument.
	for i, model := range c.models {
		if model.Key != "" {
			continue
		}
		taken := model.Menu == "inherit" || RetiredRole(MenuPrefix+model.Menu) ||
			slices.ContainsFunc(legacy.Models, func(m Model) bool { return m.Key == model.Menu })
		if taken {
			c.models[i].Menu = ""
		}
	}
	return c, skipped, nil
}

// Models returns a copy of the list in account order.
func (c *Catalogue) Models() []Model {
	if c == nil {
		return nil
	}
	out := slices.Clone(c.models)
	for i := range out {
		out[i].Efforts = slices.Clone(out[i].Efforts)
	}
	return out
}

// ByID returns the account's model with this ID.
func (c *Catalogue) ByID(id string) (Model, bool) {
	if c == nil {
		return Model{}, false
	}
	for _, model := range c.models {
		if model.ID == id {
			model.Efforts = slices.Clone(model.Efforts)
			return model, true
		}
	}
	return Model{}, false
}

// Efforts is every effort some model accepts, cheapest first, for the places that can only
// hold one list (the Agent schema, receipt labels). An effort outside a model's own set is
// still refused for that model rather than clamped.
func (c *Catalogue) Efforts() []string {
	var all []string
	for _, effort := range lowToMax {
		if c != nil && slices.ContainsFunc(c.models, func(m Model) bool { return slices.Contains(m.Efforts, effort) }) {
			all = append(all, effort)
		}
	}
	return all
}
