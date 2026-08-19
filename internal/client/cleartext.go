package client

import (
	"fmt"
	"net"
	"net/url"
)

// lookupHost resolves a name to addresses. A var so a test can state what a name
// resolves to instead of depending on the resolver of the machine it runs on.
var lookupHost = func(host string) ([]net.IP, error) { return net.LookupIP(host) }

// cgnat is 100.64.0.0/10, the shared address space of RFC 6598 — and the range
// Tailscale assigns. net.IP.IsPrivate does not cover it, so without this the
// warning below would fire on the very setup deployment.md recommends.
var _, cgnat, _ = net.ParseCIDR("100.64.0.0/10")

// cleartextWarning returns what to tell the operator when the token would cross
// an untrusted network in the clear, and "" when it would not
// (docs/design/cleartext-token-warning.md, #581).
//
// The token rides the Authorization header on every request, and PostToolUse is
// installed by default, so it is several per turn. deployment.md: "The shared
// token is only meaningful over TLS on a public network […] without TLS it is
// captured on the wire, and whoever captures it gets full read/write on the API."
//
// **Not simply "the host is not loopback."** That was the rule this started from
// and it fires on what deployment.md blesses: plain HTTP on a trusted network is
// "fine", and a private overlay — Tailscale, WireGuard, no public port at all —
// is what it *recommends* for changing IPs. A warning that fires on the
// recommended configuration is ignored, and teaches the operator to ignore the
// next one. So: public addresses only.
//
// It is a warning and never a refusal. Plain HTTP on a network the operator
// trusts is a legitimate deployment, and which networks those are is theirs to
// know, not ours.
func cleartextWarning(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil || u.Scheme != "http" {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	// Any public address is enough: a name answering with both a private and a
	// public address is reachable over the public one, and that is the path the
	// token would take.
	for _, ip := range addressesOf(host) {
		if isPublic(ip) {
			// Consequence first, then the evidence for it. The two values sit on
			// lines of their own: a URL and an address have no length anyone can
			// wrap around, and mixing one into a prose line makes the wrap depend
			// on how long the operator's host name happens to be — which is how
			// this block first came out at 96 columns.
			return fmt.Sprintf(
				"warning: this machine's token will travel in the clear.\n"+
					"         server:  %s\n"+
					"         address: %s (public)\n"+
					"         Plain HTTP to a public address means the token rides every\n"+
					"         request where it can be captured, and whoever captures it\n"+
					"         gets full read/write on the API.\n"+
					"         Put TLS in front of the daemon, or use a private network.\n"+
					"         See docs/deployment.md, \"Public exposure\".",
				serverURL, ip)
		}
	}
	return ""
}

// addressesOf resolves host, or reads it as a literal address.
//
// A resolution failure yields nothing, so nothing is said: a warning that cannot
// be justified is noise, and `init` already refuses a server it cannot reach.
func addressesOf(host string) []net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}
	}
	ips, err := lookupHost(host)
	if err != nil {
		return nil
	}
	return ips
}

// isPublic reports whether traffic to ip plausibly crosses a network the
// operator does not control. IsPrivate covers RFC 1918 and IPv6 ULA (fc00::/7,
// which is what a WireGuard mesh usually hands out); cgnat covers Tailscale.
func isPublic(ip net.IP) bool {
	switch {
	case ip == nil,
		ip.IsLoopback(),
		ip.IsPrivate(),
		ip.IsLinkLocalUnicast(),
		ip.IsLinkLocalMulticast(),
		ip.IsUnspecified(),
		cgnat.Contains(ip):
		return false
	}
	return true
}
