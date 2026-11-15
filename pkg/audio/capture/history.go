package capture

import (
	"errors"
	"sync"
	"time"
)

type History struct {
	rate int

	mu     sync.Mutex
	buf    []int16
	at     int
	filled bool
	total  int64
}

var (
	ErrNotYet = errors.New("capture: window not recorded yet")
	ErrGone   = errors.New("capture: window already overwritten")
)

func NewHistory(rate int, span time.Duration) *History {
	return &History{rate: rate, buf: make([]int16, max(samplesIn(span, rate), 1))}
}

func samplesIn(d time.Duration, rate int) int { return int(d/time.Millisecond) * rate / 1000 }

func (h *History) Add(frame []int16) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, s := range frame {
		h.buf[h.at] = s
		h.at++
		if h.at == len(h.buf) {
			h.at, h.filled = 0, true
		}
	}
	h.total += int64(len(frame))
}

func (h *History) Mark() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}

func (h *History) MarkBefore(d time.Duration) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return max(h.total-int64(samplesIn(d, h.rate)), 0)
}

func (h *History) Window(mark int64, before, after time.Duration) ([]int16, error) {
	from := mark - int64(samplesIn(before, h.rate))
	to := mark + int64(samplesIn(after, h.rate))

	h.mu.Lock()
	defer h.mu.Unlock()

	switch {
	case to > h.total:
		return nil, ErrNotYet
	case from < 0 || from < h.total-int64(len(h.buf)):
		return nil, ErrGone
	}
	out := make([]int16, to-from)
	start := h.at - int(h.total-from)
	if start < 0 {
		start += len(h.buf)
	}
	for i := range out {
		out[i] = h.buf[(start+i)%len(h.buf)]
	}
	return out, nil
}

func (h *History) Rate() int { return h.rate }

func (h *History) Recent(d time.Duration) []int16 {
	want := samplesIn(d, h.rate)

	h.mu.Lock()
	defer h.mu.Unlock()

	have := len(h.buf)
	if !h.filled {
		have = h.at
	}
	want = min(want, have)

	out := make([]int16, 0, want)
	start := h.at - want
	if start < 0 {
		start += len(h.buf)
	}
	for i := range want {
		out = append(out, h.buf[(start+i)%len(h.buf)])
	}
	return out
}
