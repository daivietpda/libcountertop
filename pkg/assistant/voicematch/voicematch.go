package voicematch

import (
	"context"
	"log/slog"
	"time"

	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const Span = 5 * time.Second

type Embedder interface {
	Embed(clip []int16) ([]float32, error)
}

type Matcher struct {
	speech *capture.History
	embed  Embedder
	heard  func(clip []int16, print []float32)
}

func New(
	speech *capture.History,
	embed Embedder,
	heard func(clip []int16, print []float32),
) *Matcher {
	return &Matcher{speech: speech, embed: embed, heard: heard}
}

func (m *Matcher) Spoke(at time.Time) {
	mark := m.speech.MarkBefore(time.Since(at))
	safe.Go("voice match", func() { m.match(at, mark) })
}

func (m *Matcher) match(at time.Time, mark int64) {
	window, err := wakeWindow(context.Background(), m.speech, at, mark, phraseAfter)
	if err != nil {
		slog.Warn("voice match clip unavailable", "err", err)
		return
	}
	clip, took, err := wakeClip(window)
	if err != nil {
		slog.Info("voice match skipped", "speech_ms", took.Milliseconds(), "err", err)
		return
	}
	start := time.Now()
	print, err := m.embed.Embed(clip)
	if err != nil {
		slog.Warn("voice match failed", "err", err)
		return
	}
	slog.Debug(
		"voice print",
		"ms",
		time.Since(start).Milliseconds(),
		"speech_ms",
		took.Milliseconds(),
	)
	m.heard(clip, print)
}
