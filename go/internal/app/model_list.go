package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// ErrModelList means the session could neither fetch its account's model list nor use the
// last good list of the same account. Nothing is guessed in its place: not the embedded
// legacy identities, not another account's list, not a cache whose account is unknown.
var ErrModelList = errors.New("ACCOUNT_MODEL_LIST_UNAVAILABLE")

// ModelListFacts is the provenance of the session's account list, for the exit status.
type ModelListFacts struct {
	// Source is "account" for a list fetched at start, "last-good" for the same account's
	// saved list after a failed fetch.
	Source    string   `json:"source"`
	FetchedAt string   `json:"fetchedAt,omitempty"`
	Models    int      `json:"models"`
	Skipped   []string `json:"skipped,omitempty"`
	Problems  []string `json:"problems,omitempty"`
	// FetchFailure is the closed category of a failed fetch when the last good list ran.
	FetchFailure string `json:"fetchFailure,omitempty"`
}

// modelListTimeout bounds the whole fetch, credential read included.
const modelListTimeout = 10 * time.Second

// modelSource is what the list needs from the session's transport (*upstream.Direct).
type modelSource interface {
	AccountModels(context.Context) (upstream.AccountModels, error)
	AccountKey() (string, error)
}

// accountModelList fetches the account's list through the session's own transport and
// credential provider, so the list and every later request belong to one bound account.
// A variable so this package's tests can supply a synthetic list without credentials.
var accountModelList = func(ctx context.Context, direct *upstream.Direct, home string) (*bridge.Catalogue, ModelListFacts, error) {
	return modelList(ctx, direct, home)
}

func modelList(ctx context.Context, direct modelSource, home string) (*bridge.Catalogue, ModelListFacts, error) {
	fetch, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	list, err := direct.AccountModels(fetch)
	facts := ModelListFacts{Source: "account"}
	if err == nil {
		if home != "" {
			// A failed save only loses the fallback for a later session.
			_ = upstream.SaveAccountModels(home, list)
		}
	} else {
		facts.Source, facts.FetchFailure = "last-good", failureCategory(err)
		key, keyErr := direct.AccountKey()
		if keyErr != nil || home == "" {
			return nil, facts, fmt.Errorf("%w: %s; the account could not be confirmed for a saved list", ErrModelList, facts.FetchFailure)
		}
		list, err = upstream.LoadAccountModels(home, key)
		if err != nil {
			return nil, facts, fmt.Errorf("%w: %s; no saved list for this account", ErrModelList, facts.FetchFailure)
		}
	}
	facts.FetchedAt = list.FetchedAt.UTC().Format(time.RFC3339)
	models := make([]bridge.AccountModel, 0, len(list.Models))
	for _, m := range list.Models {
		models = append(models, bridge.AccountModel{ID: m.ID, Efforts: m.Efforts, Default: m.Default, Visible: m.Visible})
	}
	catalogue, skipped, err := bridge.NewCatalogue(models)
	facts.Skipped = skipped
	if err != nil {
		return nil, facts, fmt.Errorf("%w: the list offers no model this build can route", ErrModelList)
	}
	facts.Models = len(catalogue.Models())
	return catalogue, facts, nil
}

// failureCategory names a fetch failure without its text, which may carry details the
// exit status must not repeat.
func failureCategory(err error) string {
	var failure upstream.Failure
	switch {
	case errors.As(err, &failure):
		if failure.Status != 0 {
			return fmt.Sprintf("%s_%d", failure.Category, failure.Status)
		}
		return failure.Category
	case errors.Is(err, context.DeadlineExceeded):
		return "REQUEST_TIMEOUT"
	case errors.Is(err, upstream.ErrModelsShape), errors.Is(err, upstream.ErrModelsTooLarge):
		return "MODEL_LIST_SHAPE"
	}
	return "MODEL_LIST_FETCH_FAILED"
}
