package gateway

import (
	"context"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// processKey names one native process for the life of this gateway: its PID and creation time,
// so a reused PID is another key. The zero key is a process nobody identified.
type processKey struct {
	pid     uint32
	created int64
}

// NativeProcess identifies a native by PID and creation time in Unix nanoseconds.
type NativeProcess struct {
	PID     uint32
	Created int64
}

// MaxRoutingProofs bounds the resident's persisted process identities.
const MaxRoutingProofs = 256

// ProvedNativeProcesses reports only natives whose own model list reached this gateway.
func (g *Gateway) ProvedNativeProcesses() []NativeProcess {
	var out []NativeProcess
	g.discovered.Range(func(key, _ any) bool {
		p := key.(processKey)
		if p.pid > 0 && p.created > 0 {
			out = append(out, NativeProcess{PID: p.pid, Created: p.created})
		}
		return true
	})
	byIdentity := func(a, b NativeProcess) int {
		if a.PID < b.PID || a.PID == b.PID && a.Created < b.Created {
			return -1
		}
		if a == b {
			return 0
		}
		return 1
	}
	slices.SortFunc(out, byIdentity)
	if len(out) > MaxRoutingProofs {
		// Discovery keeps every native it ever saw, exited ones included. Keep the ones the OS
		// still confirms first, then those it cannot confirm, then the exited; within each
		// group the lower identity wins, so the snapshot is stable.
		status := g.processStatus
		if status == nil {
			status = providercert.ProcessStatus
		}
		rank := make(map[NativeProcess]int, len(out))
		for _, p := range out {
			switch status(p.PID, p.Created) {
			case providercert.ProcessAlive:
				rank[p] = 0
			case providercert.ProcessUnknown:
				rank[p] = 1
			default:
				rank[p] = 2
			}
		}
		slices.SortStableFunc(out, func(a, b NativeProcess) int { return rank[a] - rank[b] })
		out = out[:MaxRoutingProofs]
		slices.SortFunc(out, byIdentity)
	}
	return out
}

// RestoreRoutingProof continues a same-link, same-port resident's proof before its link is
// published. The caller checks that boundary; only the OS can confirm a recorded identity.
func (g *Gateway) RestoreRoutingProof(proved []NativeProcess) {
	g.restoredAt.Store(g.startedAt)
	if g.nativeImage.Load() == nil || len(proved) > MaxRoutingProofs {
		return
	}
	status := g.processStatus
	if status == nil {
		status = providercert.ProcessStatus
	}
	for _, p := range proved {
		if p.PID > 0 && p.Created > 0 && status(p.PID, p.Created) == providercert.ProcessAlive {
			g.discovered.Store(processKey{p.PID, p.Created}, struct{}{})
		}
	}
}

// NativeProcesses reports natives registered here or observed by routing discovery.
func (g *Gateway) NativeProcesses() []NativeProcess {
	known := make(map[processKey]struct{})
	if p := g.profiles; p != nil {
		p.mu.Lock()
		for key := range p.slots {
			known[key] = struct{}{}
		}
		p.mu.Unlock()
	}
	g.discovered.Range(func(key, _ any) bool {
		known[key.(processKey)] = struct{}{}
		return true
	})
	out := make([]NativeProcess, 0, len(known))
	for key := range known {
		if key == (processKey{}) {
			continue // Legacy single-session registration identifies no native process.
		}
		out = append(out, NativeProcess{PID: key.pid, Created: key.created})
	}
	return out
}

// connIdentity is what the OS says about one connection (#319 decision 18d).
type connIdentity struct {
	native processKey
	// direct: the native process itself opened the connection, not a hook or a tool under it.
	direct bool
}

type identityContext struct{}

// RequireNativeIdentity makes the native process behind every request an OS fact (#319
// decision 18d). At accept, the TCP table names the connection's process and its nearest
// ancestor running image. A hook's registration binds its session to that native; an
// inference request must come from the native itself on a session bound to it; a hook's
// routing proof needs that native's own model list. Neither a request's order nor what it
// says about itself counts. Call before native starts.
func (g *Gateway) RequireNativeIdentity(image string) { g.nativeImage.Store(&image) }

// connContext is the server's ConnContext: identify the connection's native once, at accept.
func (g *Gateway) connContext(ctx context.Context, c net.Conn) context.Context {
	image := g.nativeImage.Load()
	if image == nil {
		return ctx
	}
	identify := g.identify
	if identify == nil {
		identify = providercert.NativeOwner
	}
	owner, native, created := identify(c.RemoteAddr(), c.LocalAddr(), *image)
	id := connIdentity{}
	if native != 0 && !created.IsZero() {
		id = connIdentity{native: processKey{native, created.UnixNano()}, direct: owner == native}
	}
	return context.WithValue(ctx, identityContext{}, id)
}

// identityOf is r's connection identity, and whether identity is required at all. Required
// with a zero native means the OS could not name one: the request is refused.
func (g *Gateway) identityOf(r *http.Request) (connIdentity, bool) {
	if g.nativeImage.Load() == nil {
		return connIdentity{}, false
	}
	id, _ := r.Context().Value(identityContext{}).(connIdentity)
	return id, true
}

// refuseIdentity answers a request whose native process the OS did not name, or that must
// come from the native itself and did not.
func (g *Gateway) refuseIdentity(w http.ResponseWriter) {
	g.refuseCategory(w, http.StatusBadRequest, "NATIVE_IDENTITY_UNVERIFIED")
}

// identifyFunc is providercert.NativeOwner's shape, replaceable by tests.
type identifyFunc func(client, server net.Addr, image string) (owner, native uint32, created time.Time)

// RequestNative is what the gateway concluded about r's connection at accept: its native
// process (PID and creation time), whether the native itself opened it, and whether identity
// is required at all. For diagnostics and tests; nothing else reads it.
func (g *Gateway) RequestNative(r *http.Request) (pid uint32, created time.Time, direct, required bool) {
	id, required := g.identityOf(r)
	if id.native == (processKey{}) {
		return 0, time.Time{}, false, required
	}
	return id.native.pid, time.Unix(0, id.native.created), id.direct, required
}
