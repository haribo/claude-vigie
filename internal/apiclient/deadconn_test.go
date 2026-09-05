package apiclient

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// #732. After a suspend/resume the machine's sessions vanish from the fleet for
// about fifteen minutes. The watcher is alive, the server is reachable — a fresh
// curl answers in 49 ms — and every POST fails `context deadline exceeded`.
//
// The watcher's 5 s cadence keeps one HTTP/2 connection hot. Suspending drops the
// peer and NAT state without the client kernel noticing, and on resume every
// request multiplexes onto that same dead connection. A request's deadline resets
// its *stream*, never the connection, and Go sends no health-check ping unless
// asked — so the connection survives until the kernel gives up retransmitting,
// which is where the fifteen minutes come from. HTTP/1.1 would have healed on the
// first request; multiplexing is the amplifier.
//
// blackholeProxy reproduces exactly that: connections opened before the flip stop
// carrying bytes in either direction, with no reset — while new connections are
// forwarded, as they are after a real resume.
type blackholeProxy struct {
	ln    net.Listener
	mu    sync.Mutex
	gen   int
	conns []net.Conn
}

func newBlackholeProxy(t *testing.T, backend string) *blackholeProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &blackholeProxy{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			gen := p.gen
			p.conns = append(p.conns, c)
			p.mu.Unlock()
			go p.serve(c, backend, gen)
		}
	}()
	return p
}

func (p *blackholeProxy) serve(client net.Conn, backend string, gen int) {
	up, err := net.Dial("tcp", backend)
	if err != nil {
		_ = client.Close()
		return
	}
	defer func() { _ = up.Close() }()

	pipe := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			p.mu.Lock()
			dead := gen < p.gen
			p.mu.Unlock()
			if dead {
				// Swallow whatever arrives and never forward it: the far side is
				// gone, and nothing tells this end so.
				if err != nil {
					return
				}
				continue
			}
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	go pipe(up, client)
	pipe(client, up)
}

// cut makes every connection opened so far stop carrying bytes. Later ones are
// forwarded normally.
func (p *blackholeProxy) cut() {
	p.mu.Lock()
	p.gen++
	p.mu.Unlock()
}

func (p *blackholeProxy) addr() string { return p.ln.Addr().String() }

func TestAClientRedialsAfterItsConnectionDies(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	proxy := newBlackholeProxy(t, srv.Listener.Addr().String())

	// The server's own client trusts its certificate; the transport under test is
	// otherwise ours, tuned the way production is.
	tr := srv.Client().Transport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a certificate this test just generated
	TuneForDeadConnections(tr)
	client := &http.Client{Transport: tr}

	url := "https://" + proxy.addr() + "/api/report"
	do := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.Body.Close()
	}

	if err := do(); err != nil {
		t.Fatalf("the first request failed before anything was cut: %v", err)
	}
	if proto := "h2"; srv.Client().Transport.(*http.Transport).ForceAttemptHTTP2 != true {
		t.Fatalf("this test only means something over %s", proto)
	}

	proxy.cut()

	// The request riding the dead connection cannot succeed — that is the outage.
	if err := do(); err == nil {
		t.Fatal("a request on a dead connection succeeded; the proxy is not cutting")
	}

	// This is the whole issue: the *next* one must not ride it too. Without a
	// health check the connection is only dropped when the kernel gives up, about
	// fifteen minutes later.
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := do()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("still on the dead connection after 20s: %v — the fleet loses this machine until the kernel gives up", err)
		}
	}
}
