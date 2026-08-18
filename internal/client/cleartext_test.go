package client

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// #581. The token rides the Authorization header on every request — several per
// turn, since PostToolUse is installed by default — so a plain-HTTP server on an
// untrusted network leaks it continuously. deployment.md says so; `vigie init`,
// the path taken by someone who has not read deployment.md, said nothing.
//
// The rule is narrower than "not loopback", and the cases below are why.
// deployment.md calls plain HTTP on a trusted network fine, and *recommends* a
// private overlay (Tailscale, WireGuard) with no public port at all. A warning
// that fires on the recommended setup is ignored, and teaches the operator to
// ignore the next one.
func TestTheWarningFiresOnlyWhenTheTokenCouldBeCaptured(t *testing.T) {
	for _, c := range []struct {
		name   string
		url    string
		resolv map[string][]string
		want   bool
	}{
		{name: "loopback literal", url: "http://127.0.0.1:8080"},
		{name: "loopback name", url: "http://localhost:8080", resolv: map[string][]string{"localhost": {"127.0.0.1"}}},
		{name: "ipv6 loopback", url: "http://[::1]:8080"},
		{name: "home LAN — deployment.md calls this fine", url: "http://192.168.1.10:8080"},
		{name: "private 10/8", url: "http://10.0.0.5:8080"},
		{name: "private 172.16/12", url: "http://172.20.0.9:8080"},
		{name: "tailscale — deployment.md RECOMMENDS this", url: "http://100.101.102.103:8080"},
		{name: "ipv6 ULA, as wireguard often hands out", url: "http://[fd00::1]:8080"},
		{name: "a LAN name", url: "http://nas.lan:8080", resolv: map[string][]string{"nas.lan": {"192.168.1.10"}}},

		{name: "a public literal", url: "http://203.0.113.5:8080", want: true},
		{name: "a public name", url: "http://vigie.example.com", resolv: map[string][]string{"vigie.example.com": {"203.0.113.5"}}, want: true},
		{name: "public ipv6", url: "http://[2001:db8::1]:8080", want: true},

		{name: "https is never a warning", url: "https://vigie.example.com"},
		{name: "https to a public literal", url: "https://203.0.113.5:8080"},

		{name: "a name that will not resolve stays silent", url: "http://nowhere.invalid:8080"},
		{name: "a url that will not parse stays silent", url: "http://[::1"},
		{name: "an empty url stays silent", url: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			restore := stubLookup(t, c.resolv)
			defer restore()
			got := cleartextWarning(c.url)
			if (got != "") != c.want {
				t.Errorf("warning = %q, want fired=%v", got, c.want)
			}
			if c.want && !strings.Contains(got, "token") {
				t.Errorf("the warning does not mention the token: %q", got)
			}
		})
	}
}

// A warning must point somewhere the operator can act on, or it is a scold.
func TestTheWarningPointsAtTheDeploymentGuide(t *testing.T) {
	restore := stubLookup(t, nil)
	defer restore()
	got := cleartextWarning("http://203.0.113.5:8080")
	if !strings.Contains(got, "deployment") {
		t.Errorf("the warning names no place to read: %q", got)
	}
}

func stubLookup(t *testing.T, resolv map[string][]string) func() {
	t.Helper()
	original := lookupHost
	lookupHost = func(host string) ([]net.IP, error) {
		names, ok := resolv[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var out []net.IP
		for _, n := range names {
			out = append(out, net.ParseIP(n))
		}
		return out, nil
	}
	return func() { lookupHost = original }
}

// TestTheWarningFitsATerminal: the block is hand-wrapped, so an edit to the
// prose can silently push a line past the fold and leave the operator reading a
// ragged paragraph. Its first form did exactly that — 96 columns, once the host
// name was long enough — which is why the server URL and the address now sit on
// lines of their own and only those two may run over.
func TestTheWarningFitsATerminal(t *testing.T) {
	const long = "http://vigie.a-rather-long-hostname.internal.example.com:8080"
	restore := stubLookup(t, map[string][]string{"vigie.a-rather-long-hostname.internal.example.com": {"203.0.113.5"}})
	defer restore()

	w := cleartextWarning(long)
	if w == "" {
		t.Fatal("no warning to measure")
	}
	// The URL's line is exempt from the 80-column rule but not from the reason it
	// is exempt: it must carry the URL and a short label, nothing else. Skipping
	// any line that merely *contains* the URL was the first form of this check,
	// and it let prose move back onto that line untouched — the exact regression
	// it exists to catch.
	const labelRoom = 20 // "         server:  "
	for _, line := range strings.Split(w, "\n") {
		if strings.Contains(line, long) {
			if rest := len([]rune(line)) - len([]rune(long)); rest > labelRoom {
				t.Errorf("the URL shares its line with %d columns of other text, so the wrap "+
					"depends on how long the operator's host name is:\n%s", rest, line)
			}
			continue
		}
		if n := len([]rune(line)); n > 80 {
			t.Errorf("line is %d columns, over 80:\n%s", n, line)
		}
	}
}
