package gateway

import (
	"net/http"
	"sync/atomic"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

type startupSelection struct {
	main      verifiedStartup
	residents map[string]*verifiedStartup
	permits   func(bridge.Pair) bool
}

type verifiedStartup struct {
	pair     bridge.Pair
	verified atomic.Bool
}

// ConfigureStartupSelection binds the launch decision before native can submit requests.
func (g *Gateway) ConfigureStartupSelection(pair bridge.Pair, permits func(bridge.Pair) bool) {
	g.startup.main.pair, g.startup.permits = pair, permits
}

func (g *Gateway) pendingStartup(r *http.Request) *verifiedStartup {
	if r.Header.Get("X-Claude-Code-Agent-Id") != "" || r.Header.Get("X-Claude-Code-Request-Class") != "main" {
		return nil
	}
	start := g.startup.residents[r.Header.Get("X-Claude-Code-Session-Id")]
	if start == nil {
		start = &g.startup.main
	}
	if start.pair == (bridge.Pair{}) || start.verified.Load() {
		return nil
	}
	return start
}

func (g *Gateway) matchesStartup(start *verifiedStartup, request *anthropic.Request, effective bridge.Pair) bool {
	if effective != start.pair {
		return false
	}
	if request.Model == effective.Model {
		return true
	}
	route, err := g.selection.SelectRoute(request.Model, "")
	return err == nil && route.Source == "catalogue" && route.Model == effective.Model
}

func (g *Gateway) routePermitted(pair bridge.Pair) bool {
	return g.startup.permits == nil || g.startup.permits(pair)
}
