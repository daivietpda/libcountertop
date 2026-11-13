package rtsp

import (
	"encoding/binary"
	"time"
)

const (
	payloadType      = 96
	audioPayloadType = 97
	maxPayload       = 1400
	headerLen        = 12
)

type packetizer struct {
	kind   byte
	ssrc   uint32
	seq    uint16
	sent   uint32
	octets uint32
}

func (p *packetizer) packets(nalus [][]byte, ts uint32) [][]byte {
	var out [][]byte
	for i, n := range nalus {
		last := i == len(nalus)-1
		if len(n) <= maxPayload {
			out = append(out, p.packet(n, nil, ts, last))
			continue
		}
		indicator := n[0]&0xe0 | 28
		kind := n[0] & 0x1f
		body := n[1:]
		first := true
		for len(body) > 0 {
			take := min(len(body), maxPayload-2)
			header := kind
			if first {
				header |= 0x80
			}
			end := take == len(body)
			if end {
				header |= 0x40
			}
			out = append(out, p.packet([]byte{indicator, header}, body[:take], ts, last && end))
			body = body[take:]
			first = false
		}
	}
	return out
}

// RFC 3640 AAC-hbr: a 16-bit AU-headers-length, then one 13-bit size and 3-bit index per AU.
func (p *packetizer) aac(au []byte, ts uint32) []byte {
	n := len(au)
	head := []byte{0x00, 0x10, byte(n >> 5), byte(n&0x1f) << 3}
	return p.packet(head, au, ts, true)
}

func (p *packetizer) packet(head, body []byte, ts uint32, marker bool) []byte {
	b := make([]byte, headerLen, headerLen+len(head)+len(body))
	b[0] = 0x80
	b[1] = p.kind
	if marker {
		b[1] |= 0x80
	}
	binary.BigEndian.PutUint16(b[2:], p.seq)
	binary.BigEndian.PutUint32(b[4:], ts)
	binary.BigEndian.PutUint32(b[8:], p.ssrc)
	p.seq++
	p.sent++
	p.octets += uint32(len(head) + len(body))
	b = append(b, head...)
	return append(b, body...)
}

const (
	rtcpSR    = 200
	rtcpSDES  = 202
	ntpToUnix = 2208988800
)

func (p *packetizer) report(at time.Time, ts uint32, cname string) []byte {
	b := make([]byte, 28)
	b[0] = 0x80
	b[1] = rtcpSR
	binary.BigEndian.PutUint16(b[2:], 6)
	binary.BigEndian.PutUint32(b[4:], p.ssrc)
	binary.BigEndian.PutUint32(b[8:], uint32(at.Unix()+ntpToUnix))
	binary.BigEndian.PutUint32(b[12:], uint32(uint64(at.Nanosecond())<<32/1e9))
	binary.BigEndian.PutUint32(b[16:], ts)
	binary.BigEndian.PutUint32(b[20:], p.sent)
	binary.BigEndian.PutUint32(b[24:], p.octets)

	cname = cname[:min(len(cname), 255)]
	chunk := 4 + 2 + len(cname) + 1
	chunk = (chunk + 3) &^ 3
	sdes := make([]byte, 4+chunk)
	sdes[0] = 0x81
	sdes[1] = rtcpSDES
	binary.BigEndian.PutUint16(sdes[2:], uint16(len(sdes)/4-1))
	binary.BigEndian.PutUint32(sdes[4:], p.ssrc)
	sdes[8] = 1
	sdes[9] = byte(len(cname))
	copy(sdes[10:], cname)
	return append(b, sdes...)
}
