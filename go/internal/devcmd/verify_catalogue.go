package devcmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// catalogueDirect builds the Direct transport of --verify-catalogue-refusal from the fixture account
// and token it is given. The credential store is the in-memory fixture: only <home>/auth.json is
// readable, and nothing is refreshed. internal/app builds no provider itself.
func catalogueDirect(home, account, token string) *upstream.Direct {
	provider := &auth.Provider{Home: home, Environ: map[string]string{}, ReadFile: func(name string) ([]byte, error) {
		if name != filepath.Join(home, "auth.json") {
			return nil, os.ErrNotExist
		}
		return []byte(fmt.Sprintf("{\"auth_mode\":\"chatgpt\",\"tokens\":{\"account_id\":%q,\"access_token\":%q}}", account, token)), nil
	}}
	return upstream.NewDirect(provider, upstream.NewLedger(upstream.Budget{}), upstream.Fixed("0.160.0"))
}
