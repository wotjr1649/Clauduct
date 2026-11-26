package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type catalogueVerificationTransport func(*http.Request) (*http.Response, error)

func (f catalogueVerificationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// VerifyCatalogueRefusal exercises the launcher's model-list boundary without
// native children, OS credential storage or non-loopback network access.
func VerifyCatalogueRefusal(ctx context.Context) ([]map[string]any, error) {
	home, err := os.MkdirTemp("", "clauduct-catalogue-verification-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	const account = "clauduct-verification-account"
	if err := upstream.SaveAccountModels(home, upstream.AccountModels{
		Account: upstream.AccountKey(account), ClientVersion: "0.160.0", FetchedAt: time.Now().UTC(),
		Models: []upstream.AccountModel{{ID: "gpt-6-luna", Efforts: []string{"low"}, Default: "low", Visible: true}},
	}); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(home, ".clauduct", "account-models.json")
	saved, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(saved))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	endpoint := listener.Addr().String()
	var targetStatus, requests, accepted, blocked atomic.Int64
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/backend-api/codex/models" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(400)
			return
		}
		requests.Add(1)
		w.WriteHeader(int(targetStatus.Load()))
	})}}
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != endpoint || (network != "tcp" && network != "tcp4") {
			blocked.Add(1)
			return nil, errors.New("catalogue verification dial outside fixture")
		}
		accepted.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp4", endpoint)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if connection, err := transport.DialContext(ctx, "tcp", "192.0.2.1:443"); connection != nil || err == nil {
		if connection != nil {
			connection.Close()
		}
		return nil, errors.New("catalogue verification external dial control failed")
	}
	token := "fixture." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("{\"exp\":%d}", time.Now().Add(time.Hour).Unix()))) + ".fixture"
	provider := &auth.Provider{Home: home, Environ: map[string]string{}, ReadFile: func(name string) ([]byte, error) {
		if name != filepath.Join(home, "auth.json") {
			return nil, os.ErrNotExist
		}
		return []byte(fmt.Sprintf("{\"auth_mode\":\"chatgpt\",\"tokens\":{\"account_id\":%q,\"access_token\":%q}}", account, token)), nil
	}}
	direct := upstream.NewDirect(provider, upstream.NewLedger(upstream.Budget{}), upstream.Fixed("0.160.0"))
	var observedStatus atomic.Int64
	direct.Client = &http.Client{CheckRedirect: client.CheckRedirect, Transport: catalogueVerificationTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Host != "chatgpt.com" ||
			r.URL.Path != "/backend-api/codex/models" || r.Header.Get("Authorization") != "Bearer "+token {
			return nil, errors.New("catalogue verification rejected unexpected request")
		}
		req := r.Clone(r.Context())
		req.URL.Host, req.Host = endpoint, endpoint
		req.Header = make(http.Header)
		response, err := client.Do(req)
		if err == nil {
			observedStatus.Store(int64(response.StatusCode))
		}
		return response, err
	})}
	stop := errors.New("catalogue verification stopped before native resolution")
	rows := make([]map[string]any, 0, 3)
	for _, status := range []int{401, 403, 503} {
		targetStatus.Store(int64(status))
		initial := requests.Load()
		gatewayStarts, nativeStarts, nativeLookups := 0, 0, 0
		var facts ModelListFacts
		_, launchErr := Run(ctx, Options{
			Args: []string{"-p", "--model", "gpt-6-luna", "--effort", "low", "catalogue verification"},
			Env:  map[string]string{"USERPROFILE": home, "HOME": home, "CLAUDE_CONFIG_DIR": home},
			Cwd:  home, ClauductHome: home, Stdout: io.Discard, Stderr: io.Discard,
			ManagedPolicy: func() ([]policyDocument, error) { return nil, nil },
			ResolveClaude: func() (string, bool, error) { nativeLookups++; return "", false, stop },
			StartGateway:  func() (*gateway.Gateway, error) { gatewayStarts++; return nil, stop },
			StartProcess:  func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error) { nativeStarts++; return nil, stop },
			ModelList: func(ctx context.Context, home string) (*bridge.Catalogue, ModelListFacts, error) {
				result, f, e := AccountModelList(ctx, direct, home)
				facts = f
				return result, f, e
			},
		})
		final, err := os.ReadFile(cachePath)
		if err != nil {
			return nil, err
		}
		finalDigest := fmt.Sprintf("%x", sha256.Sum256(final))
		category := fmt.Sprintf("MODELS_HTTP_ERROR_%d", status)
		if finalDigest != digest || observedStatus.Load() != int64(status) || requests.Load() != initial+1 ||
			blocked.Load() != 1 || accepted.Load() < 1 || facts.FetchFailure != category || nativeStarts+gatewayStarts != 0 {
			return nil, fmt.Errorf("catalogue verification transport, cache or startup mismatch at %d", status)
		}
		categoryOf := ErrModelList.Error()
		if status == 503 {
			if !errors.Is(launchErr, stop) || facts.Source != "last-good" || nativeLookups != 1 {
				return nil, errors.New("catalogue verification last-good control did not reach its boundary")
			}
			categoryOf = "LAST_GOOD_CONFIRMED"
		} else if !errors.Is(launchErr, ErrModelList) || !strings.Contains(launchErr.Error(), category) || nativeLookups != 0 {
			return nil, fmt.Errorf("catalogue verification refusal mismatch at %d", status)
		}
		rows = append(rows, map[string]any{
			"httpStatus": status, "category": categoryOf, "fetchFailure": facts.FetchFailure,
			"cacheBefore": digest, "cacheAfter": finalDigest, "source": facts.Source,
			"nativeLookups": nativeLookups, "nativeStarts": nativeStarts, "gatewayStarts": gatewayStarts,
			"loopback": endpoint, "loopbackDials": accepted.Load(), "deniedDials": blocked.Load(), "requests": requests.Load(),
		})
	}
	return rows, nil
}
