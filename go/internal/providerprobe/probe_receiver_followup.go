package providerprobe

import (
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

type p3ProxyConnectionKey struct {
	id       int64
	server   string
	accepted int64
}

func p3ProxyConnection(connection *providercert.ReceiverConnection) (p3ProxyConnectionKey, bool) {
	if connection == nil || connection.ID <= 0 || connection.Server == "" || connection.Accepted.IsZero() {
		return p3ProxyConnectionKey{}, false
	}
	return p3ProxyConnectionKey{id: connection.ID, server: connection.Server, accepted: connection.Accepted.UnixNano()}, true
}

// p3ProxyContacts counts catch-all connections naming target that are not already represented
// by the same accepted connection in target.Observer. The destination scheme is ignored for
// contact accounting: a wrong-scheme attempt is still a contact. Only same-port loopback
// aliases are matched; nothing is resolved or dialed.
func p3ProxyContacts(seen []providercert.Seen, targetURL string, targetSeen []providercert.Seen) int64 {
	registered := map[p3ProxyConnectionKey]struct{}{}
	for _, record := range targetSeen {
		if key, ok := p3ProxyConnection(record.Connection); ok {
			registered[key] = struct{}{}
		}
	}
	counted := map[p3ProxyConnectionKey]struct{}{}
	var contacts int64
	for _, record := range seen {
		if !p3AuthorityNamesReceiver(record.Authority, targetURL) {
			continue
		}
		key, ok := p3ProxyConnection(record.Connection)
		if !ok {
			// Missing connection metadata cannot prove reuse, so count conservatively.
			contacts++
			continue
		}
		if _, exists := registered[key]; exists {
			// The target observer already contributed this in-process route to the current
			// receiver's contact count.
			continue
		}
		if _, exists := counted[key]; exists {
			continue
		}
		counted[key] = struct{}{}
		contacts++
	}
	return contacts
}

// p3ProxyClassified proves that every accepted catch-all connection produced a kept observation
// through the catch-all or an owned in-process receiver. Completed CONNECT tunnels need an
// inner HTTP request; a tunnel that closes before its first request remains unclassified.
func p3ProxyClassified(proxyURL string, proxy providercert.ReceiverSnapshot, targets ...providercert.ReceiverSnapshot) bool {
	if !proxy.Complete || proxy.Dropped != 0 {
		return false
	}
	address, err := url.Parse(proxyURL)
	if err != nil || address.Scheme != "http" || address.Host == "" {
		return false
	}
	records := slices.Clone(proxy.Seen)
	for _, target := range targets {
		if !target.Complete || target.Dropped != 0 {
			return false
		}
		records = append(records, target.Seen...)
	}
	accepted := proxy.Conns
	if accepted < 0 {
		return false
	}
	connections := map[int64]p3ProxyConnectionKey{}
	finishedTLS := map[int64]bool{}
	innerHTTP := map[int64]bool{}
	for _, record := range records {
		if record.Undecodable {
			return false
		}
		connection := record.Connection
		if connection == nil || connection.Server != address.Host {
			continue
		}
		key, ok := p3ProxyConnection(connection)
		if !ok || connection.Client == "" || connection.Owner.PID == 0 || connection.Owner.Created.IsZero() || connection.ID > accepted {
			return false
		}
		if prior, exists := connections[connection.ID]; exists && prior != key {
			return false
		}
		connections[connection.ID] = key
		if record.Method == http.MethodConnect {
			if !record.TLSRejected {
				finishedTLS[connection.ID] = true
			}
		} else {
			innerHTTP[connection.ID] = true
		}
	}
	if int64(len(connections)) != accepted {
		return false
	}
	for id := int64(1); id <= accepted; id++ {
		if _, ok := connections[id]; !ok {
			return false
		}
	}
	for id := range finishedTLS {
		if !innerHTTP[id] {
			return false
		}
	}
	return true
}

func p3AuthorityNamesReceiver(authority, receiverURL string) bool {
	u, err := url.Parse(receiverURL)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	ownerHost, ownerPort, err := net.SplitHostPort(u.Host)
	if err != nil || !p3LoopbackHost(ownerHost) {
		return false
	}
	seenHost, seenPort, err := net.SplitHostPort(authority)
	if err != nil || !p3LoopbackHost(seenHost) {
		return false
	}
	ownerPortNumber, err := strconv.Atoi(ownerPort)
	if err != nil || ownerPortNumber < 1 || ownerPortNumber > 65535 {
		return false
	}
	seenPortNumber, err := strconv.Atoi(seenPort)
	if err != nil || seenPortNumber != ownerPortNumber {
		return false
	}
	if p3AuthorityHost(ownerHost) == p3AuthorityHost(seenHost) {
		return true
	}
	if p3Localhost(ownerHost) || p3Localhost(seenHost) {
		return true
	}
	ownerIP, seenIP := net.ParseIP(ownerHost), net.ParseIP(seenHost)
	return ownerIP != nil && seenIP != nil && ownerIP.Equal(seenIP)
}

func p3LoopbackHost(host string) bool {
	if p3Localhost(host) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func p3Localhost(host string) bool {
	return strings.EqualFold(strings.TrimSuffix(host, "."), "localhost")
}

func p3AuthorityHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
