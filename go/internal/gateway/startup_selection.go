package gateway

import (
	"net/http"
	"sync/atomic"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

type startupSelection struct {
	main    verifiedStartup
	resolve func(session, cwd string, native NativeProcess) (bridge.Pair, func(bridge.Pair) bool, error)
}

type verifiedStartup struct {
	pair     bridge.Pair
	permits  func(bridge.Pair) bool
	verified atomic.Bool
}

// ConfigureStartupSelection binds the launch decision before native can submit requests.
func (g *Gateway) ConfigureStartupSelection(pair bridge.Pair, permits func(bridge.Pair) bool) {
	g.startup.main.pair, g.startup.main.permits = pair, permits
}

// ConfigureTaskStartup resolves independently dispatched sessions using their native launch.
func (g *Gateway) ConfigureTaskStartup(resolve func(string, string, NativeProcess) (bridge.Pair, func(bridge.Pair) bool, error)) {
	g.startup.resolve = resolve
}

func (g *Gateway) sessionStartup(r *http.Request) *verifiedStartup {
	if p := g.profiles; p != nil {
		p.mu.Lock()
		start := p.starts[r.Header.Get("X-Claude-Code-Session-Id")]
		p.mu.Unlock()
		return start
	}
	return &g.startup.main
}

func (g *Gateway) pendingStartup(r *http.Request) *verifiedStartup {
	if r.Header.Get("X-Claude-Code-Agent-Id") != "" || r.Header.Get("X-Claude-Code-Request-Class") != "main" {
		return nil
	}
	start := g.sessionStartup(r)
	if start == nil || start.pair == (bridge.Pair{}) || start.verified.Load() {
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

func (g *Gateway) routePermitted(r *http.Request, pair bridge.Pair) bool {
	start := g.sessionStartup(r)
	if start == nil {
		start = &g.startup.main
	}
	return start.permits == nil || start.permits(pair)
}
