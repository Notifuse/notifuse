package testutil

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// LatencyProxy is a minimal TCP passthrough proxy used to reproduce
// connection-manager churn deterministically in integration tests. It forwards
// every accepted connection to a fixed upstream address and can inject two
// independent, controllable kinds of latency:
//
//   - Dial latency: a delay before establishing each NEW upstream connection.
//     This makes brand-new physical connections slow while existing/idle
//     pooled connections stay fast — useful to prove that creating one
//     workspace's pool must not block access to a different, already-cached
//     workspace.
//
//   - Data latency: a delay before forwarding each chunk in either direction.
//     This makes round-trips on ESTABLISHED connections slow — useful to make a
//     pool health-check ping exceed its timeout on demand.
//
// Both knobs are safe to toggle from another goroutine while the proxy runs.
type LatencyProxy struct {
	listener     net.Listener
	upstreamAddr string
	dialLatency  atomic.Int64 // nanoseconds
	dataLatency  atomic.Int64 // nanoseconds
	blackhole    atomic.Bool
	wg           sync.WaitGroup
	closeOnce    sync.Once
	done         chan struct{}

	connMu sync.Mutex
	active map[net.Conn]struct{} // live endpoints, for blackhole teardown
}

// NewLatencyProxy starts a proxy listening on a random loopback port that
// forwards to upstreamAddr (e.g. "127.0.0.1:5433"). Call Close when done.
func NewLatencyProxy(upstreamAddr string) (*LatencyProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &LatencyProxy{
		listener:     ln,
		upstreamAddr: upstreamAddr,
		done:         make(chan struct{}),
		active:       make(map[net.Conn]struct{}),
	}
	p.wg.Add(1)
	go p.acceptLoop()
	return p, nil
}

// Host returns the loopback host the proxy listens on.
func (p *LatencyProxy) Host() string {
	return p.listener.Addr().(*net.TCPAddr).IP.String()
}

// Port returns the port the proxy listens on.
func (p *LatencyProxy) Port() int {
	return p.listener.Addr().(*net.TCPAddr).Port
}

// SetDialLatency sets the delay applied before dialing upstream for each new
// connection. Pass 0 to disable.
func (p *LatencyProxy) SetDialLatency(d time.Duration) { p.dialLatency.Store(int64(d)) }

// SetDataLatency sets the delay applied before forwarding each chunk of data in
// either direction. Pass 0 to disable.
func (p *LatencyProxy) SetDataLatency(d time.Duration) { p.dataLatency.Store(int64(d)) }

// SetBlackhole toggles a hard outage: while enabled, all in-flight connections
// are severed and every new connection is accepted and immediately reset,
// without reaching upstream. This forces a genuine connection error (as opposed
// to mere slowness) — the condition that makes a pool health-check ping fail.
func (p *LatencyProxy) SetBlackhole(on bool) {
	p.blackhole.Store(on)
	if on {
		p.connMu.Lock()
		for c := range p.active {
			_ = c.Close()
		}
		p.connMu.Unlock()
	}
}

func (p *LatencyProxy) track(c net.Conn) {
	p.connMu.Lock()
	p.active[c] = struct{}{}
	p.connMu.Unlock()
}

func (p *LatencyProxy) untrack(c net.Conn) {
	p.connMu.Lock()
	delete(p.active, c)
	p.connMu.Unlock()
}

func (p *LatencyProxy) acceptLoop() {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return // listener closed
		}
		p.wg.Add(1)
		go p.handle(conn)
	}
}

func (p *LatencyProxy) isClosed() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *LatencyProxy) handle(client net.Conn) {
	defer p.wg.Done()
	defer func() { _ = client.Close() }()

	if p.blackhole.Load() || p.isClosed() {
		return // accept then immediately reset
	}

	p.track(client)
	defer p.untrack(client)

	if d := time.Duration(p.dialLatency.Load()); d > 0 {
		select {
		case <-time.After(d):
		case <-p.done:
			return
		}
	}

	if p.blackhole.Load() || p.isClosed() {
		return
	}

	upstream, err := net.Dial("tcp", p.upstreamAddr)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()
	p.track(upstream)
	defer p.untrack(upstream)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.pipe(upstream, client) }() // client -> upstream
	go func() { defer wg.Done(); p.pipe(client, upstream) }() // upstream -> client
	wg.Wait()
}

func (p *LatencyProxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if d := time.Duration(p.dataLatency.Load()); d > 0 {
				select {
				case <-time.After(d):
				case <-p.done:
					return
				}
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			// Propagate the half-close so the peer observes EOF.
			if c, ok := dst.(*net.TCPConn); ok {
				_ = c.CloseWrite()
			}
			return
		}
	}
}

// Close stops accepting new connections, severs any in-flight connections, and
// waits for all proxy goroutines to exit so nothing lingers past the test.
func (p *LatencyProxy) Close() error {
	p.closeOnce.Do(func() {
		close(p.done)
		_ = p.listener.Close()
		p.connMu.Lock()
		for c := range p.active {
			_ = c.Close()
		}
		p.connMu.Unlock()
	})
	p.wg.Wait()
	return nil
}
