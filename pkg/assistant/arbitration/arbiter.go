// Package arbitration lets the device that heard a wake word best answer it, when several heard it.
package arbitration

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/peer"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	Port      = 6070
	Lookback  = schema.MaxArbitration * time.Millisecond
	readBytes = 1024
)

type Options struct {
	ID     func() string
	Port   int
	Key    func() string
	Peers  func() []peer.Peer
	Wait   func() time.Duration
	Offset func() float64
}

type watch struct {
	claim  Claim
	beaten chan Claim
}

type heard struct {
	claim Claim
	at    time.Time
}

type Arbiter struct {
	o Options

	mu      sync.Mutex
	conn    *net.UDPConn
	home    string
	key     []byte
	counter uint64
	last    map[string]uint64
	recent  []heard
	watches []*watch
}

func New(o Options) *Arbiter {
	return &Arbiter{o: o, last: map[string]uint64{}}
}

func (a *Arbiter) Name() string { return "arbitration" }

func (a *Arbiter) Run(ctx context.Context) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: a.o.Port})
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() {
		a.mu.Lock()
		a.conn = nil
		a.mu.Unlock()
		_ = conn.Close()
	}()

	buf := make([]byte, readBytes)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		a.receive(buf[:n], from)
	}
}

func (a *Arbiter) Decide(word string, score float64) bool {
	wait := a.o.Wait()
	if wait <= 0 {
		return true
	}
	if a.o.Offset != nil {
		score += a.o.Offset()
	}
	mine := Claim{Word: fold(word), Score: score, From: a.o.ID()}

	a.mu.Lock()
	now := time.Now()
	for _, h := range a.recent {
		if h.claim.Word == mine.Word && now.Sub(h.at) <= Lookback && h.claim.beats(mine) {
			a.mu.Unlock()
			slog.Info(
				"wake answered elsewhere",
				"by",
				h.claim.From,
				"score",
				h.claim.Score,
				"ours",
				score,
			)
			return false
		}
	}
	w := &watch{claim: mine, beaten: make(chan Claim, 1)}
	a.watches = append(a.watches, w)
	a.counter = max(a.counter+1, uint64(now.UnixNano()))
	mine.counter = a.counter
	conn, key := a.conn, a.keyLocked()
	a.mu.Unlock()
	defer a.unwatch(w)

	if conn != nil {
		a.send(conn, seal(key, mine))
	}

	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case by := <-w.beaten:
		slog.Info("wake answered elsewhere", "by", by.From, "score", by.Score, "ours", score)
		return false
	case <-t.C:
		slog.Info("wake arbitration won", "score", score, "wait_ms", wait.Milliseconds())
		return true
	}
}

func (a *Arbiter) send(conn *net.UDPConn, b []byte) {
	self := a.o.ID()
	for _, p := range a.o.Peers() {
		if p.ID == self || p.Arbitration == 0 || !p.Standing.Trusted() {
			continue
		}
		ip := slices.IndexFunc(p.Addrs, func(ip net.IP) bool { return ip.To4() != nil })
		if ip < 0 {
			continue
		}
		to := &net.UDPAddr{IP: p.Addrs[ip], Port: p.Arbitration}
		if _, err := conn.WriteToUDP(b, to); err != nil {
			slog.Debug("sending a wake claim failed", "peer", p.Name, "err", err)
		}
	}
}

func (a *Arbiter) receive(b []byte, from *net.UDPAddr) {
	a.mu.Lock()
	defer a.mu.Unlock()

	c, err := open(a.keyLocked(), b)
	if err != nil {
		slog.Debug("dropped a wake claim", "from", from, "err", err)
		return
	}
	if c.From == a.o.ID() || c.counter <= a.last[c.From] {
		return
	}
	a.last[c.From] = c.counter
	slog.Debug("wake claim", "from", c.From, "word", c.Word, "score", c.Score)

	now := time.Now()
	a.recent = slices.DeleteFunc(a.recent, func(h heard) bool { return now.Sub(h.at) > Lookback })
	a.recent = append(a.recent, heard{claim: c, at: now})

	for _, w := range a.watches {
		if w.claim.Word == c.Word && c.beats(w.claim) {
			select {
			case w.beaten <- c:
			default:
			}
		}
	}
}

func (a *Arbiter) unwatch(w *watch) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.watches = slices.DeleteFunc(a.watches, func(x *watch) bool { return x == w })
}

func (a *Arbiter) keyLocked() []byte {
	if home := a.o.Key(); a.key == nil || home != a.home {
		a.home, a.key = home, derive(home)
	}
	return a.key
}

func fold(word string) string { return strings.ToLower(strings.TrimSpace(word)) }
