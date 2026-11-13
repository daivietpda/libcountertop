package rtsp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

type AAC struct {
	Rate     int
	Channels int
	Config   []byte
}

type Stream struct {
	OnDemand func()
	Audio    *AAC

	mu       sync.Mutex
	sps, pps []byte
	subs     map[*session]bool
	ready    chan struct{}
}

func NewStream() *Stream { return &Stream{subs: map[*session]bool{}, ready: make(chan struct{})} }

func (st *Stream) Write(au []byte, pts time.Duration) {
	var body [][]byte
	key := false
	st.mu.Lock()
	for _, n := range NALUs(au) {
		switch nalType(n) {
		case nalAUD:
			continue
		case nalSPS:
			if !bytes.Equal(st.sps, n) {
				st.sps = append([]byte(nil), n...)
			}
		case nalPPS:
			if !bytes.Equal(st.pps, n) {
				st.pps = append([]byte(nil), n...)
			}
		case nalIDR:
			key = true
		}
		body = append(body, n)
	}
	subs := make([]*session, 0, len(st.subs))
	for s := range st.subs {
		subs = append(subs, s)
	}
	sps, pps := st.sps, st.pps
	if len(sps) >= 4 && len(pps) > 0 {
		select {
		case <-st.ready:
		default:
			close(st.ready)
		}
	}
	st.mu.Unlock()

	if len(body) == 0 {
		return
	}
	for _, s := range subs {
		s.deliver(body, key, sps, pps, pts)
	}
}

func (st *Stream) WriteAudio(au []byte, pts time.Duration) {
	if st.Audio == nil || len(au) == 0 {
		return
	}
	for _, s := range st.sessions() {
		s.deliverAudio(au, st.Audio.Rate, pts)
	}
}

func (st *Stream) sessions() []*session {
	st.mu.Lock()
	defer st.mu.Unlock()
	subs := make([]*session, 0, len(st.subs))
	for s := range st.subs {
		subs = append(subs, s)
	}
	return subs
}

func (st *Stream) demand() {
	if st.OnDemand != nil {
		st.OnDemand()
	}
}

func (st *Stream) add(s *session) {
	st.mu.Lock()
	st.subs[s] = true
	st.mu.Unlock()
}

func (st *Stream) remove(s *session) {
	st.mu.Lock()
	delete(st.subs, s)
	st.mu.Unlock()
}

func (st *Stream) Viewers() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.subs)
}

func (st *Stream) sdpNamed(id uint32, host string, wait time.Duration, name string) (string, bool) {
	select {
	case <-st.ready:
	case <-time.After(wait):
		return "", false
	}
	st.mu.Lock()
	sps, pps := st.sps, st.pps
	st.mu.Unlock()
	if len(sps) < 4 || len(pps) == 0 {
		return "", false
	}
	enc := base64.StdEncoding.EncodeToString
	sdp := fmt.Sprintf(
		"v=0\r\n"+
			"o=- %d 1 IN IP4 %s\r\n"+
			"s=%s\r\n"+
			"c=IN IP4 0.0.0.0\r\n"+
			"t=0 0\r\n"+
			"m=video 0 RTP/AVP %d\r\n"+
			"a=rtpmap:%d H264/90000\r\n"+
			"a=fmtp:%d packetization-mode=1;profile-level-id=%02X%02X%02X;sprop-parameter-sets=%s,%s\r\n"+
			"a=control:trackID=0\r\n",
		id,
		host,
		name,
		payloadType,
		payloadType,
		payloadType,
		sps[1],
		sps[2],
		sps[3],
		enc(sps),
		enc(pps),
	)
	if a := st.Audio; a != nil {
		sdp += fmt.Sprintf(
			"m=audio 0 RTP/AVP %d\r\n"+
				"a=rtpmap:%d MPEG4-GENERIC/%d/%d\r\n"+
				"a=fmtp:%d streamtype=5;profile-level-id=1;mode=AAC-hbr;"+
				"sizelength=13;indexlength=3;indexdeltalength=3;config=%X\r\n"+
				"a=control:trackID=1\r\n",
			audioPayloadType,
			audioPayloadType,
			a.Rate,
			a.Channels,
			audioPayloadType,
			a.Config,
		)
	}
	return sdp, true
}
