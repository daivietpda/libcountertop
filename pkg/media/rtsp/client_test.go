package rtsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTheClientPlaysWhatTheServerSends(t *testing.T) {
	_, st, addr := serve(t)
	st.Write(annexB(sps, pps, slice(nalIDR, 100)), 0)

	units := make(chan Unit, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, "rtsp://"+addr+"/main", Options{Video: func(u Unit) { units <- u }})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() { _ = c.Run(ctx) }()

	big := slice(nalIDR, 4000)
	go func() {
		for i := 1; ctx.Err() == nil; i++ {
			st.Write(annexB(big), time.Duration(i)*40*time.Millisecond)
			time.Sleep(20 * time.Millisecond)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("no fragmented keyframe arrived")
		case u := <-units:
			if u.Codec != H264 || !u.Key {
				continue
			}
			if bytes.Contains(u.Data, big) {
				return
			}
		}
	}
}

func TestH265FragmentsReassemble(t *testing.T) {
	idr := []byte{19 << 1, 1, 0xaa, 0xbb, 0xcc, 0xdd}
	d := &depacketizer{codec: H265}
	fu := func(start, end bool, body ...byte) []byte {
		h := byte(19)
		if start {
			h |= 0x80
		}
		if end {
			h |= 0x40
		}
		return append([]byte{h265FU << 1, 1, h}, body...)
	}
	if _, _, done := d.push(fu(true, false, 0xaa, 0xbb), false); done {
		t.Fatal("a unit finished before its last fragment")
	}
	au, key, done := d.push(fu(false, true, 0xcc, 0xdd), true)
	if !done || !key {
		t.Fatalf("done %v key %v, want a finished keyframe", done, key)
	}
	if want := append([]byte{0, 0, 0, 1}, idr...); !bytes.Equal(au, want) {
		t.Fatalf("got % x, want % x", au, want)
	}
}

func TestBackchannelUsesTheChannelTheServerAssigns(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	sdp := "v=0\r\n" +
		"m=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\na=control:trackID=0\r\na=recvonly\r\n" +
		"m=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=control:trackID=1\r\na=recvonly\r\n" +
		"m=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=control:trackID=2\r\na=sendonly\r\n"
	got := make(chan byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		tp := textproto.NewReader(r)
		for {
			if b, err := r.Peek(1); err == nil && b[0] == '$' {
				head := make([]byte, 4)
				if _, err := io.ReadFull(r, head); err != nil {
					return
				}
				body := make([]byte, binary.BigEndian.Uint16(head[2:]))
				if _, err := io.ReadFull(r, body); err != nil {
					return
				}
				if head[1]&1 == 0 {
					got <- head[1]
					return
				}
				continue
			}
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			h, err := tp.ReadMIMEHeader()
			if err != nil {
				return
			}
			method, uri, _ := strings.Cut(line, " ")
			uri, _, _ = strings.Cut(uri, " ")
			reply := "RTSP/1.0 200 OK\r\nCSeq: " + h.Get("CSeq") + "\r\nSession: 1\r\n"
			switch method {
			case "DESCRIBE":
				reply += fmt.Sprintf(
					"Content-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s",
					len(sdp),
					sdp,
				)
			case "SETUP":
				track, _ := strconv.Atoi(uri[strings.LastIndexByte(uri, '=')+1:])
				reply += fmt.Sprintf(
					"Transport: RTP/AVP/TCP;unicast;interleaved=%d-%d\r\n\r\n",
					track*2,
					track*2+1,
				)
			default:
				reply += "\r\n"
			}
			if _, err := io.WriteString(conn, reply); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(
		ctx,
		"rtsp://"+ln.Addr().String()+"/door?backchannel=1",
		Options{Backchannel: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.SendBack(make([]byte, 160), 160); err != nil {
		t.Fatal(err)
	}
	select {
	case ch := <-got:
		if ch != 4 {
			t.Fatalf("backchannel audio went out on channel %d, want 4", ch)
		}
	case <-ctx.Done():
		t.Fatal("the server got no backchannel audio")
	}
}

func TestDescriptionFindsTheBackchannel(t *testing.T) {
	m := parseSDP("v=0\r\nm=video 0 RTP/AVP 96\r\na=rtpmap:96 H265/90000\r\na=control:track1\r\n" +
		"m=audio 0 RTP/AVP 0\r\na=control:track3\r\na=sendonly\r\n")
	if len(m) != 2 || m[0].codec() != H265 || m[0].control != "track1" {
		t.Fatalf("video %+v", m)
	}
	if !m[1].send || m[1].encoding != "PCMU" || m[1].rate != 8000 {
		t.Fatalf("backchannel %+v", m[1])
	}
}
