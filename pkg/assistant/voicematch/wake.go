package voicematch

import (
	"context"
	"errors"
	"time"

	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/audio/vad"
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

const (
	wakeBefore  = 1500 * time.Millisecond
	phraseAfter = 3 * time.Second
	voiced      = 0.5
	shortest    = 250 * time.Millisecond
	frame       = 20 * time.Millisecond
	retries     = 10
)

var ErrNoSpeech = errors.New("voicematch: no speech heard")

func wakeWindow(
	ctx context.Context,
	h *capture.History,
	at time.Time,
	mark int64,
	after time.Duration,
) ([]int16, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Until(at.Add(after + frame))):
	}
	w, err := h.Window(mark, wakeBefore, after)
	for try := 0; errors.Is(err, capture.ErrNotYet) && try < retries; try++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(frame):
		}
		w, err = h.Window(mark, wakeBefore, after)
	}
	return w, err
}

func wakeClip(window []int16) ([]int16, time.Duration, error) {
	d, err := vad.New()
	if err != nil {
		return nil, 0, err
	}
	n := len(window) / vad.Chunk
	speech := make([]bool, n)
	for i := range n {
		p, err := d.Speech(window[i*vad.Chunk : (i+1)*vad.Chunk])
		if err != nil {
			return nil, 0, err
		}
		speech[i] = p >= voiced
	}
	var kept []int16
	for i := range n {
		if speech[i] || (i > 0 && speech[i-1]) || (i+1 < n && speech[i+1]) {
			kept = append(kept, window[i*vad.Chunk:(i+1)*vad.Chunk]...)
		}
	}
	took := time.Duration(len(kept)) * time.Second / voiceprint.SampleRate
	if took < shortest {
		return nil, took, ErrNoSpeech
	}
	clip := make([]int16, voiceprint.ClipSamples)
	for i := range clip {
		clip[i] = kept[i%len(kept)]
	}
	return clip, took, nil
}
