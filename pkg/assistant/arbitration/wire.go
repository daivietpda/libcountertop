package arbitration

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
)

const (
	version  = 1
	tagBytes = 16
	keyBytes = 32
	label    = "countertop wake arbitration 1"
)

var errClaim = errors.New("arbitration: not a claim from this home")

type Claim struct {
	Word  string
	Score float64
	From  string

	counter uint64
}

func (c Claim) beats(o Claim) bool {
	if c.Score != o.Score {
		return c.Score > o.Score
	}
	return c.From < o.From
}

func derive(home string) []byte {
	k, err := hkdf.Key(sha256.New, []byte(home), nil, label, keyBytes)
	if err != nil {
		panic(err)
	}
	return k
}

func seal(key []byte, c Claim) []byte {
	b := []byte{version}
	b = binary.BigEndian.AppendUint64(b, c.counter)
	b = binary.BigEndian.AppendUint32(b, math.Float32bits(float32(c.Score)))
	b = appendText(b, c.From)
	b = appendText(b, c.Word)
	return append(b, tag(key, b)...)
}

func open(key, b []byte) (Claim, error) {
	if len(b) < 1+8+4+1+1+tagBytes || b[0] != version {
		return Claim{}, errClaim
	}
	body, sum := b[:len(b)-tagBytes], b[len(b)-tagBytes:]
	if !hmac.Equal(sum, tag(key, body)) {
		return Claim{}, errClaim
	}
	c := Claim{
		counter: binary.BigEndian.Uint64(body[1:]),
		Score:   float64(math.Float32frombits(binary.BigEndian.Uint32(body[9:]))),
	}
	rest := body[13:]
	var ok bool
	if c.From, rest, ok = readText(rest); !ok {
		return Claim{}, errClaim
	}
	if c.Word, rest, ok = readText(rest); !ok || len(rest) != 0 {
		return Claim{}, errClaim
	}
	return c, nil
}

func tag(key, body []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(body)
	return m.Sum(nil)[:tagBytes]
}

func appendText(b []byte, s string) []byte {
	if len(s) > math.MaxUint8 {
		s = s[:math.MaxUint8]
	}
	b = append(b, byte(len(s)))
	return append(b, s...)
}

func readText(b []byte) (string, []byte, bool) {
	if len(b) < 1 || len(b) < 1+int(b[0]) {
		return "", nil, false
	}
	n := int(b[0])
	return string(b[1 : 1+n]), b[1+n:], true
}
