package arbitration

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/peer"
)

func TestAClaimRoundTripsOnlyUnderTheHomeKey(t *testing.T) {
	c := Claim{Word: "hey jarvis", Score: 21.5, From: "kitchen", counter: 7}
	b := seal(derive("home"), c)

	got, err := open(derive("home"), b)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("got %+v, want %+v", got, c)
	}
	if _, err := open(derive("other home"), b); err == nil {
		t.Error("a claim opened under another home's key")
	}
	b[5] ^= 1
	if _, err := open(derive("home"), b); err == nil {
		t.Error("a changed claim opened")
	}
}

func TestLouderBeatsQuieterAndTiesGoToTheLowerID(t *testing.T) {
	loud, quiet := Claim{Score: 20, From: "b"}, Claim{Score: 10, From: "a"}
	if !loud.beats(quiet) || quiet.beats(loud) {
		t.Error("the louder claim did not win")
	}
	a, b := Claim{Score: 10, From: "a"}, Claim{Score: 10, From: "b"}
	if !a.beats(b) || b.beats(a) {
		t.Error("a tie did not go to the lower id")
	}
}

func TestNoWaitDecidesAtOnce(t *testing.T) {
	a := New(Options{ID: id("a"), Key: home, Peers: none, Wait: func() time.Duration { return 0 }})
	began := time.Now()
	if !a.Decide("hey jarvis", 1) {
		t.Fatal("a device with no wait stood down")
	}
	if time.Since(began) > 10*time.Millisecond {
		t.Error("a device with no wait waited")
	}
}

func TestTheQuieterDeviceStandsDown(t *testing.T) {
	near, far := pair(t, 200*time.Millisecond)

	var nearWon, farWon bool
	var wg sync.WaitGroup
	wg.Go(func() { nearWon = near.Decide("Hey Jarvis", 30) })
	wg.Go(func() { farWon = far.Decide("hey jarvis", 12) })
	wg.Wait()

	if !nearWon || farWon {
		t.Fatalf("near won %v, far won %v; want only the near one", nearWon, farWon)
	}
}

func TestAnEarlierBetterClaimEndsTheWaitAtOnce(t *testing.T) {
	near, far := pair(t, time.Second)

	go near.Decide("hey jarvis", 30)
	deadline := time.Now().Add(time.Second)
	for len(recent(far)) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	began := time.Now()
	if far.Decide("hey jarvis", 12) {
		t.Fatal("the quieter device answered")
	}
	if time.Since(began) > 50*time.Millisecond {
		t.Error("the quieter device waited out its wait")
	}
}

func TestAnotherWakeWordDoesNotCompete(t *testing.T) {
	near, far := pair(t, 100*time.Millisecond)

	var nearWon, farWon bool
	var wg sync.WaitGroup
	wg.Go(func() { nearWon = near.Decide("hey jarvis", 30) })
	wg.Go(func() { farWon = far.Decide("okay nabu", 12) })
	wg.Wait()

	if !nearWon || !farWon {
		t.Fatalf("near won %v, far won %v; want both", nearWon, farWon)
	}
}

func TestAReplayedClaimIsDropped(t *testing.T) {
	_, far := pair(t, 100*time.Millisecond)
	b := seal(derive("home"), Claim{Word: "hey jarvis", Score: 30, From: "near", counter: 5})
	far.receive(b, nil)
	far.receive(b, nil)
	if n := len(recent(far)); n != 1 {
		t.Fatalf("%d claims kept, want 1", n)
	}
}

func pair(t *testing.T, wait time.Duration) (near, far *Arbiter) {
	t.Helper()
	var nearPeers, farPeers []peer.Peer
	near = New(Options{
		ID: id("near"), Key: home, Wait: func() time.Duration { return wait },
		Peers: func() []peer.Peer { return nearPeers },
	})
	far = New(Options{
		ID: id("far"), Key: home, Wait: func() time.Duration { return wait },
		Peers: func() []peer.Peer { return farPeers },
	})
	nearPort, farPort := serve(t, near), serve(t, far)
	nearPeers = []peer.Peer{loopback("far", farPort)}
	farPeers = []peer.Peer{loopback("near", nearPort)}
	return near, far
}

func serve(t *testing.T, a *Arbiter) int {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = a.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		conn := a.conn
		a.mu.Unlock()
		if conn != nil {
			return conn.LocalAddr().(*net.UDPAddr).Port
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the arbiter did not listen")
	return 0
}

func loopback(id string, port int) peer.Peer {
	return peer.Peer{
		ID:          id,
		Name:        id,
		Addrs:       []net.IP{net.IPv4(127, 0, 0, 1)},
		Arbitration: port,
		Standing:    peer.Home,
	}
}

func TestAPeerThatAdvertisesNoPortIsSentNothing(t *testing.T) {
	near, far := pair(t, 100*time.Millisecond)
	silent := loopback("far", 0)
	silent.Arbitration = 0
	near.o.Peers = func() []peer.Peer { return []peer.Peer{silent} }

	var farWon bool
	var wg sync.WaitGroup
	wg.Go(func() { near.Decide("hey jarvis", 30) })
	wg.Go(func() { farWon = far.Decide("hey jarvis", 12) })
	wg.Wait()

	if !farWon {
		t.Fatal("a claim reached a peer that advertises no arbitration port")
	}
}

func recent(a *Arbiter) []heard {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]heard(nil), a.recent...)
}

func TestAnOffsetCanTurnTheResult(t *testing.T) {
	near, far := pair(t, 200*time.Millisecond)
	far.o.Offset = func() float64 { return 20 }

	var nearWon, farWon bool
	var wg sync.WaitGroup
	wg.Go(func() { nearWon = near.Decide("hey jarvis", 30) })
	wg.Go(func() { farWon = far.Decide("hey jarvis", 12) })
	wg.Wait()

	if nearWon || !farWon {
		t.Fatalf("near won %v, far won %v; want the far one, 12+20 over 30", nearWon, farWon)
	}
}

func id(s string) func() string { return func() string { return s } }

func home() string { return "home" }

func none() []peer.Peer { return nil }
