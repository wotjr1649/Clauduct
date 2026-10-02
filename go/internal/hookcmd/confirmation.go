package hookcmd

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

const ConfirmationArg = "--clauduct-confirmation"

// ConfirmationStale is the helper's exit code when the gateway no longer waits
// for its nonce: the request ended first. The module closes it without failing.
const ConfirmationStale = 3

// No shell, argv payload, filesystem, credential store, redirect, proxy or retry.
// Native supplies a bounded identity proof on stdin and owns this child's life.
func confirmation(in io.Reader, out io.Writer, env map[string]string) int {
	raw, err := io.ReadAll(io.LimitReader(in, 4097))
	if err != nil || len(raw) > 4096 {
		return 2
	}
	if _, err = wire.Fields(raw, []string{"nonce", "session", "agent", "turn", "index", "search", "auxiliary", "confirmations", "unmatched"}); err != nil {
		return 2
	}
	base, token := env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_AUTH_TOKEN"]
	if !loopback.MatchString(base) || token == "" {
		return 2
	}
	address, err := url.Parse(base)
	if err != nil {
		return 2
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil || port < 1 || port > 65535 {
		return 2
	}
	var dialed atomic.Bool
	client := &http.Client{Transport: &http.Transport{
		ResponseHeaderTimeout: requestTimeout,
		DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
			if target != address.Host || dialed.Swap(true) {
				return nil, errInvalidGateway
			}
			return (&net.Dialer{Timeout: requestTimeout}).DialContext(ctx, network, target)
		},
	}, CheckRedirect: func(*http.Request, []*http.Request) error { return errInvalidGateway }}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(http.MethodPost, base+"/clauduct/confirmation", bytes.NewReader(raw))
	if err != nil {
		return 2
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return 2
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusGone {
		return ConfirmationStale
	}
	if res.StatusCode != http.StatusOK {
		return 2
	}
	const marker = "CLAUDUCT_CONFIRMATION_CONNECTED\n"
	head := make([]byte, len(marker))
	if _, err = io.ReadFull(res.Body, head); err != nil || string(head) != marker {
		return 2
	}
	if _, err = io.WriteString(out, marker); err != nil {
		return 2
	}
	var tail [1]byte
	if n, err := res.Body.Read(tail[:]); n != 0 || err != io.EOF {
		return 2
	}
	return 0
}
