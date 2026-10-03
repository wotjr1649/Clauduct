package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// The account's model list is read from the same backend, under the same credential, as an
// inference: one GET beside /responses, no body, no stream, no model turn. It is not an
// inference and is never reserved against the ledger.

const modelsSuffix = "/models"

// Bounds on one model list read. Nothing here retries: a failure is reported and the caller
// falls back to the last list it saved.
const (
	accountModelsTimeout = 10 * time.Second
	maxModelsBytes       = 4 << 20
	maxAccountModels     = 512
	maxModelEfforts      = 32
)

var (
	modelSlug    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
	effortName   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	wholeVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})(?:[-+][0-9A-Za-z.+-]*)?$`)
	storedWhole  = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
)

// Refusals particular to the model list.
var (
	// ErrModelsEndpoint means the configured endpoint does not name a path a model list
	// address can be derived from. Refused rather than guessed.
	ErrModelsEndpoint = errors.New("MODELS_ENDPOINT_INVALID")
	// ErrModelsTooLarge means the reply exceeded the size bound.
	ErrModelsTooLarge = errors.New("MODELS_RESPONSE_TOO_LARGE")
	// ErrModelsShape means the reply is not a list this build can trust whole. The reason
	// that follows it is fixed text; nothing from the reply is echoed.
	ErrModelsShape = errors.New("MODELS_RESPONSE_SHAPE")
)

// ModelsHTTPError is the Failure category of a model list reply other than 200. The
// Failure carries the status number and nothing from the body.
const ModelsHTTPError = "MODELS_HTTP_ERROR"

// AccountModel is one model the account may use, reduced to what routing needs. Efforts
// are the backend's strings as sent; choosing which of them can be transmitted is the
// caller's decision.
type AccountModel struct {
	ID      string
	Efforts []string
	Default string
	Visible bool
}

// AccountModels is one account's model list. Account is AccountKey of the credential's
// account, never the account id itself.
type AccountModels struct {
	Account       string
	ClientVersion string
	FetchedAt     time.Time
	Models        []AccountModel
}

type modelListCounters struct{ requests, failures atomic.Int64 }

// ModelListStats counts model list reads for diagnostics. They are not inferences and
// appear nowhere in the ledger.
type ModelListStats struct {
	Requests int64 `json:"requests"`
	Failures int64 `json:"failures"`
}

func (d *Direct) ModelListStats() ModelListStats {
	return ModelListStats{d.modelCounts.requests.Load(), d.modelCounts.failures.Load()}
}

// AccountKey is the identifier a model list is stored and compared under: the SHA-256 of
// the account id in lower-case hex, so the id itself is never written.
func AccountKey(account string) string {
	sum := sha256.Sum256([]byte(account))
	return hex.EncodeToString(sum[:])
}

// AccountKey reads the current credential through the provider, which binds the session to
// its account exactly as a request would, and returns that account's key. Nothing is sent.
func (d *Direct) AccountKey() (string, error) {
	if d.Credentials == nil {
		return "", &auth.Error{Category: auth.CategoryUnavailable}
	}
	credential, err := d.Credentials.Credential()
	if err != nil {
		return "", err
	}
	return AccountKey(credential.Account), nil
}

func (d *Direct) modelsTarget() (string, error) {
	target := d.target()
	if !strings.HasSuffix(target, responsesSuffix) {
		return "", ErrModelsEndpoint
	}
	return strings.TrimSuffix(target, responsesSuffix) + modelsSuffix, nil
}

func (d *Direct) modelsTimeout() time.Duration {
	if d.modelsFor > 0 {
		return d.modelsFor
	}
	return accountModelsTimeout
}

// clientVersionWhole is the MAJOR.MINOR.PATCH the reference client sends as client_version.
func clientVersionWhole(version string) (string, error) {
	match := wholeVersion.FindStringSubmatch(version)
	if match == nil {
		return "", ErrVersionInvalid
	}
	return match[1] + "." + match[2] + "." + match[3], nil
}

// AccountModels reads the account's model list once.
//
// Only a 200 whose whole list validates succeeds; anything else is an error that names a
// category and, for a status, its number -- never the token, the account or the reply.
func (d *Direct) AccountModels(ctx context.Context) (result AccountModels, err error) {
	d.modelCounts.requests.Add(1)
	defer func() {
		if err != nil {
			d.modelCounts.failures.Add(1)
		}
	}()
	target, err := d.modelsTarget()
	if err != nil {
		return AccountModels{}, err
	}
	if d.Credentials == nil {
		return AccountModels{}, &auth.Error{Category: auth.CategoryUnavailable}
	}
	credential, err := d.Credentials.Credential()
	if err != nil {
		return AccountModels{}, err
	}
	if credential.Synthetic {
		return AccountModels{}, ErrSyntheticMixing
	}
	version, err := d.clientVersion()
	if err != nil {
		return AccountModels{}, err
	}
	whole, err := clientVersionWhole(version)
	if err != nil {
		return AccountModels{}, err
	}
	address, err := url.Parse(target)
	if err != nil {
		return AccountModels{}, ErrModelsEndpoint
	}
	address.RawQuery = url.Values{"client_version": {whole}}.Encode()

	ctx, cancel := context.WithTimeout(ctx, d.modelsTimeout())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return AccountModels{}, ErrModelsEndpoint
	}
	applyIdentity(request, credential, version)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")

	response, err := send(d.client(), request)
	if err != nil {
		return AccountModels{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AccountModels{}, Failure{Category: ModelsHTTPError, Disposition: Terminal, Status: response.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxModelsBytes+1))
	if err != nil {
		return AccountModels{}, ClassifyTransport(err)
	}
	if len(raw) > maxModelsBytes {
		return AccountModels{}, ErrModelsTooLarge
	}
	models, err := parseAccountModels(raw)
	if err != nil {
		return AccountModels{}, err
	}
	return AccountModels{
		Account:       AccountKey(credential.Account),
		ClientVersion: whole,
		FetchedAt:     time.Now().UTC(),
		Models:        models,
	}, nil
}

func shapeError(reason string) error { return fmt.Errorf("%w: %s", ErrModelsShape, reason) }

// parseAccountModels reads {"models":[...]}, keeping only slug, the effort strings,
// default_reasoning_level and visibility. Other fields are scanned but never decoded or kept; a needed field
// of the wrong type, a repeated key, or one bad model refuses the whole list.
func parseAccountModels(raw []byte) ([]AccountModel, error) {
	top, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, shapeError("document")
	}
	list, presence := wire.Of(top, "models")
	var entries []json.RawMessage
	if presence != wire.Present || json.Unmarshal(list, &entries) != nil {
		return nil, shapeError("models")
	}
	models := make([]AccountModel, 0, len(entries))
	for _, entry := range entries {
		model, err := parseAccountModel(entry)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	if err := validateAccountModels(models); err != nil {
		return nil, err
	}
	return models, nil
}

func parseAccountModel(raw json.RawMessage) (AccountModel, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return AccountModel{}, shapeError("model")
	}
	var model AccountModel
	if model.ID, err = requiredString(fields, "slug"); err != nil {
		return AccountModel{}, err
	}
	visibility, err := requiredString(fields, "visibility")
	if err != nil {
		return AccountModel{}, err
	}
	model.Visible = visibility == "list"
	if value, presence := wire.Of(fields, "default_reasoning_level"); presence == wire.Present {
		if json.Unmarshal(value, &model.Default) != nil {
			return AccountModel{}, shapeError("default_reasoning_level")
		}
	}
	// Read, not filtered on: in ChatGPT mode the reference client keeps every model.
	if value, presence := wire.Of(fields, "supported_in_api"); presence == wire.Present {
		var supported bool
		if json.Unmarshal(value, &supported) != nil {
			return AccountModel{}, shapeError("supported_in_api")
		}
	}
	value, presence := wire.Of(fields, "supported_reasoning_levels")
	var levels []json.RawMessage
	if presence != wire.Present || json.Unmarshal(value, &levels) != nil {
		return AccountModel{}, shapeError("supported_reasoning_levels")
	}
	model.Efforts = make([]string, 0, len(levels))
	for _, level := range levels {
		levelFields, err := wire.Fields(level, nil)
		if err != nil {
			return AccountModel{}, shapeError("reasoning level")
		}
		effort, err := requiredString(levelFields, "effort")
		if err != nil {
			return AccountModel{}, err
		}
		model.Efforts = append(model.Efforts, effort)
	}
	return model, nil
}

func requiredString(fields map[string]json.RawMessage, name string) (string, error) {
	value, presence := wire.Of(fields, name)
	var out string
	if presence != wire.Present || json.Unmarshal(value, &out) != nil {
		return "", shapeError(name)
	}
	return out, nil
}

// validateAccountModels is the one rule set a fetched list and a stored list both meet.
func validateAccountModels(models []AccountModel) error {
	if len(models) == 0 || len(models) > maxAccountModels {
		return shapeError("model count")
	}
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		if !modelSlug.MatchString(model.ID) {
			return shapeError("slug")
		}
		if seen[model.ID] {
			return shapeError("duplicate slug")
		}
		seen[model.ID] = true
		if len(model.Efforts) > maxModelEfforts {
			return shapeError("effort count")
		}
		efforts := make(map[string]bool, len(model.Efforts))
		for _, effort := range model.Efforts {
			if !effortName.MatchString(effort) {
				return shapeError("effort")
			}
			if efforts[effort] {
				return shapeError("duplicate effort")
			}
			efforts[effort] = true
		}
		if model.Default != "" && !effortName.MatchString(model.Default) {
			return shapeError("default_reasoning_level")
		}
	}
	return nil
}
