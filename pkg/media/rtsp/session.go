package rtsp

import (
	"encoding/binary"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

const (
	queued = 256

	// RFC 3550 6.2 puts the minimum RTCP interval at 5 s.
	reportEvery = 5 * time.Second
)

const (
	videoTrack = iota
	audioTrack
	trackCount
)

type packet struct {
	b       []byte
	track   int
	control bool
}

type track struct {
	pz       packetizer
	base     uint32
	zero     time.Duration
	zeroed   bool
	open     bool
	channel  byte
	udp      *net.UDPConn
	rtcp     *net.UDPConn
	peer     *net.UDPAddr
	reported time.Time
}

func (t *track) release() {
	if t.udp != nil {
		_ = t.udp.Close()
	}
	if t.rtcp != nil {
		_ = t.rtcp.Close()
	}
	*t = track{pz: t.pz, base: t.base}
}

func (t *track) stamp(pts time.Duration, rate int64) uint32 {
	if !t.zeroed {
		t.zero, t.zeroed = pts, true
	}
	return t.base + uint32(int64(pts-t.zero)*rate/int64(time.Second))
}

type session struct {
	cnamePrefix string
	id          string
	stream      *Stream
	tcp         *conn

	mu      sync.Mutex
	tracks  [trackCount]track
	started bool
	playing bool
	out     chan packet
	done    chan struct{}
	stop    sync.Once
}

func newSession(id string) *session {
	s := &session{
		id:          id,
		cnamePrefix: "countertop-",
		out:         make(chan packet, queued),
		done:        make(chan struct{}),
	}
	kinds := [trackCount]byte{payloadType, audioPayloadType}
	for i := range s.tracks {
		s.tracks[i] = track{
			pz:   packetizer{kind: kinds[i], ssrc: rand.Uint32(), seq: uint16(rand.Uint32())},
			base: rand.Uint32(),
		}
	}
	return s
}

func (s *session) play() {
	s.mu.Lock()
	if s.playing {
		s.mu.Unlock()
		return
	}
	s.playing = true
	s.mu.Unlock()
	go s.send()
	s.stream.add(s)
}

func (s *session) close() {
	s.stop.Do(func() {
		if s.stream != nil {
			s.stream.remove(s)
		}
		close(s.done)
		s.mu.Lock()
		for i := range s.tracks {
			s.tracks[i].release()
		}
		s.mu.Unlock()
	})
}

func (s *session) deliver(nalus [][]byte, key bool, sps, pps []byte, pts time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &s.tracks[videoTrack]
	if !t.open {
		return
	}
	if !s.started {
		if !key {
			return
		}
		s.started = true
	}
	if key && nalType(nalus[0]) != nalSPS && len(sps) > 0 && len(pps) > 0 {
		nalus = append([][]byte{sps, pps}, nalus...)
	}
	ts := t.stamp(pts, 90000)
	for _, p := range t.pz.packets(nalus, ts) {
		if !s.queue(packet{b: p, track: videoTrack}) {
			return
		}
	}
	s.report(videoTrack, ts)
}

func (s *session) deliverAudio(au []byte, rate int, pts time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &s.tracks[audioTrack]
	if !t.open {
		return
	}
	ts := t.stamp(pts, int64(rate))
	if !s.queue(packet{b: t.pz.aac(au, ts), track: audioTrack}) {
		return
	}
	s.report(audioTrack, ts)
}

func (s *session) report(at int, ts uint32) {
	t := &s.tracks[at]
	now := time.Now()
	if now.Sub(t.reported) < reportEvery {
		return
	}
	t.reported = now
	s.queue(packet{b: t.pz.report(now, ts, s.cnamePrefix+s.id), track: at, control: true})
}

func (s *session) queue(p packet) bool {
	select {
	case s.out <- p:
		return true
	default:
		s.started = false
		return false
	}
}

func (s *session) send() {
	for {
		select {
		case <-s.done:
			return
		case p := <-s.out:
			s.mu.Lock()
			t := s.tracks[p.track]
			s.mu.Unlock()
			var err error
			switch {
			case s.tcp != nil && p.control:
				err = s.tcp.interleaved(t.channel+1, p.b)
			case s.tcp != nil:
				err = s.tcp.interleaved(t.channel, p.b)
			case t.udp == nil:
			case p.control:
				_, _ = t.rtcp.WriteToUDP(
					p.b,
					&net.UDPAddr{IP: t.peer.IP, Port: t.peer.Port + 1, Zone: t.peer.Zone},
				)
			default:
				_, err = t.udp.WriteToUDP(p.b, t.peer)
			}
			if err != nil {
				s.close()
				return
			}
		}
	}
}

func (s *session) rtpInfo(at int) (uint16, uint32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &s.tracks[at]
	return t.pz.seq, t.base, t.open
}

func frame(channel byte, p []byte) []byte {
	b := make([]byte, 4, 4+len(p))
	b[0] = '$'
	b[1] = channel
	binary.BigEndian.PutUint16(b[2:], uint16(len(p)))
	return append(b, p...)
}
