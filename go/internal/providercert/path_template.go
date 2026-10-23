package providercert

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// PathTemplate is the evidence form of one request for the boundary ledger. It is built from a
// reviewed ROUTE TABLE, not from what a value looks like: a path is shown literally only where a
// reviewed route has a literal at that position, every position a route leaves dynamic is rendered
// as its placeholder (never the value), and a request that matches no route shows no path content at
// all. Nothing a user, a project or a machine chose (a name, a repository, a file, a session title,
// a token, a host of their own) can appear in it.
//
// Canonical grammar (the JS parser mirrors exactly this):
//
//	template := METHOD " " HOST PATH
//	METHOD   := "GET" | "HEAD" | "POST" | "PUT" | "PATCH" | "DELETE" | "OPTIONS" | "CONNECT" | "{method}"
//	            the request method upper-cased; any other method is {method}
//	HOST     := a host of templateHosts | "{ip}" | "{host}"      (see Host canonicalization)
//	PATH     := ROUTE | FALLBACK
//	ROUTE    := "/" SEG { "/" SEG }      the pattern of the matching route of templateRoutes, verbatim
//	FALLBACK := "/" | { "/{seg}" } [ "/{more}" ]    (1 to 10 "/{seg}", then "/{more}" if more segments follow)
//	SEG (in a route pattern) := a literal | "{seg}" | "{uuid}" | "{num}" | "{version}" | "{hex}"
//
// Host canonicalization (strict): the authority is lower-cased. It is {host} when it is empty, has
// userinfo ("@"), or has a port that is not 1 to 5 decimal digits with a value of at most 65535
// (an empty port, "+80", "8o" and "99999" included), or when a bracketed authority is not exactly
// "[" IP "]" optionally followed by ":" PORT. A valid port is then dropped. The remaining host is
// {ip} when net.ParseIP accepts it, its own text when it is exactly a member of templateHosts, and
// {host} otherwise (a trailing dot and an IPv6 zone are not folded: lower-casing is the only folding).
//
// Path handling: the path is cut at the first "?" or "#" (a query or fragment is never read), split
// on "/", and empty segments are skipped. Each segment is percent-decoded ONCE; a segment that does
// not decode, or that decodes to "." or "..", never matches a route position that is not {seg}
// (and shows as {seg}). A route matches when it has exactly the request's number of segments, the
// host is "*" or the canonical host, and every route position accepts the decoded segment: a literal
// by exact case-sensitive equality, {seg} any text, {uuid} 8-4-4-4-12 hexadecimal digits, {num}
// decimal digits only, {version} [v]N.N.N or [v]N.N.N.N with digits only, {hex} 8 to 128 hexadecimal
// digits of one case. Among matching routes the winner has a host of its own over "*", then the
// most literal segments, then the smaller pattern text. The template shows the route's pattern, so a
// dynamic position is its placeholder whatever the request held.
//
// Bounds: a route pattern is at most 66 bytes, a fallback at most 10*6 + 7 = 67; the host is at
// most 34 bytes ("{host}" is 6), the method at most 8 ("{method}"; the known methods are at most
// 7). The longest possible template is therefore
//
//	8 + 1 + 34 + 67 = 110 bytes   (PathTemplateMaxLength)
//
// and the tests recompute it from the table and assert it.
const (
	pathTemplateMaxSegments = 10
	// PathTemplateMaxLength is the exact upper bound of len(PathTemplate(...)).
	PathTemplateMaxLength = 8 + 1 + 34 + 67
)

type templateRoute struct{ host, pattern string }

type parsedRoute struct {
	host     string
	pattern  string
	segments []string
	literals int
}

var (
	templateUUID    = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	templateHex     = regexp.MustCompile(`^(?:[0-9a-f]{8,128}|[0-9A-F]{8,128})$`)
	templateVersion = regexp.MustCompile(`^v?[0-9]+(?:\.[0-9]+){2,3}$`)
	templateNum     = regexp.MustCompile(`^[0-9]+$`)
	templatePort    = regexp.MustCompile(`^[0-9]{1,5}$`)

	hostSet = toSet(templateHosts)
	routes  = parseRoutes(templateRoutes)
)

func toSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

func parseRoutes(table []templateRoute) []parsedRoute {
	out := make([]parsedRoute, 0, len(table))
	for _, r := range table {
		segs := strings.Split(strings.TrimPrefix(r.pattern, "/"), "/")
		literals := 0
		for _, s := range segs {
			if !strings.HasPrefix(s, "{") {
				literals++
			}
		}
		out = append(out, parsedRoute{host: r.host, pattern: r.pattern, segments: segs, literals: literals})
	}
	return out
}

var templateMethods = toSet([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT"})

// PathTemplate builds the template for one request. path is the RAW request path as sent (for an
// http.Request, url.RawPath when set, else url.EscapedPath(); the observer uses rawRequestPath): each
// segment is percent-decoded exactly once here, after the split on "/", so a doubly escaped segment
// (%256d) stays a literal "%6d" and an escaped slash (%2F) stays inside its segment. Never pass the
// decoded url.Path, and never EscapedPath() of a RawPath that is not a valid encoding (it re-escapes
// the decoded path). host is the request authority (a port is accepted and dropped when it is valid).
func PathTemplate(method, host, path string) string {
	m := strings.ToUpper(method)
	if !templateMethods[m] {
		m = "{method}"
	}
	h := templateHost(host)
	return m + " " + h + templatePath(h, path)
}

// templateHost canonicalizes an authority (see the grammar).
func templateHost(authority string) string {
	a := strings.ToLower(authority)
	host, port := a, ""
	switch {
	case strings.HasPrefix(a, "["):
		end := strings.Index(a, "]")
		if end < 0 {
			return "{host}"
		}
		host = a[1:end]
		rest := a[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return "{host}"
			}
			port = rest[1:]
		}
		if net.ParseIP(host) == nil {
			return "{host}"
		}
		if rest != "" && !validPort(port) {
			return "{host}"
		}
		return "{ip}"
	case net.ParseIP(a) != nil:
		return "{ip}" // a bare IP literal, including an IPv6 address without brackets
	case strings.Contains(a, ":"):
		i := strings.LastIndex(a, ":")
		host, port = a[:i], a[i+1:]
		if !validPort(port) {
			return "{host}"
		}
	}
	if host == "" || strings.Contains(host, ":") {
		return "{host}"
	}
	if net.ParseIP(host) != nil {
		return "{ip}"
	}
	if hostSet[host] {
		return host
	}
	return "{host}"
}

func validPort(p string) bool {
	if !templatePort.MatchString(p) {
		return false
	}
	n, err := strconv.Atoi(p)
	return err == nil && n <= 65535
}

func templatePath(host, path string) string {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	var segs []string
	for _, raw := range strings.Split(path, "/") {
		if raw == "" {
			continue
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil || decoded == "." || decoded == ".." {
			decoded = "\x00" // matches only a {seg} position, never a literal or a typed class
		}
		segs = append(segs, decoded)
	}
	if route := matchRoute(host, segs); route != nil {
		return route.pattern
	}
	if len(segs) == 0 {
		return "/"
	}
	var out strings.Builder
	for i := range segs {
		if i == pathTemplateMaxSegments {
			out.WriteString("/{more}")
			break
		}
		out.WriteString("/{seg}")
	}
	return out.String()
}

func segmentAccepts(position, segment string) bool {
	switch position {
	case "{seg}":
		return true
	case "{uuid}":
		return templateUUID.MatchString(segment)
	case "{num}":
		return templateNum.MatchString(segment)
	case "{version}":
		return templateVersion.MatchString(segment)
	case "{hex}":
		return templateHex.MatchString(segment)
	}
	return position == segment
}

// matchRoute returns the winning route for the canonical host and decoded segments, or nil.
func matchRoute(host string, segs []string) *parsedRoute {
	var best *parsedRoute
	for i := range routes {
		r := &routes[i]
		if len(r.segments) != len(segs) || r.host != "*" && r.host != host {
			continue
		}
		ok := true
		for j, position := range r.segments {
			if !segmentAccepts(position, segs[j]) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if best == nil || better(r, best) {
			best = r
		}
	}
	return best
}

// better is the tie-break among matching routes: a host of its own, then more literals, then the
// smaller pattern text.
func better(a, b *parsedRoute) bool {
	if (a.host != "*") != (b.host != "*") {
		return a.host != "*"
	}
	if a.literals != b.literals {
		return a.literals > b.literals
	}
	return a.pattern < b.pattern
}
