package voiceprint

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/inference/tflite"
)

const (
	ClipSamples = 48080
	Clip        = time.Duration(ClipSamples) * time.Second / SampleRate
)

type Runner interface {
	tflite.Splitter
	Run(func())
}

type Embedder struct {
	run Runner

	mu    sync.Mutex
	front *Frontend
	model *tflite.Interpreter
	size  int
}

func New(model []byte, run Runner) (*Embedder, error) {
	m, err := tflite.Parse(model)
	if err != nil {
		return nil, err
	}
	in, err := tflite.New(m)
	if err != nil {
		return nil, err
	}
	if want := Bands * Frames(ClipSamples); in.Input(0).Count() != want {
		return nil, fmt.Errorf(
			"voiceprint: model takes %v, want %d mel values",
			in.Input(0).Shape,
			want,
		)
	}
	in.SetSplitter(run)
	return &Embedder{run: run, front: NewFrontend(), model: in, size: in.Output(0).Count()}, nil
}

func (e *Embedder) Size() int { return e.size }

func (e *Embedder) Embed(clip []int16) ([]float32, error) {
	if len(clip) != ClipSamples {
		return nil, fmt.Errorf("voiceprint: clip has %d samples, want %d", len(clip), ClipSamples)
	}
	samples := make([]float32, len(clip))
	for i, s := range clip {
		samples[i] = float32(s) / 32768
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	e.run.Run(func() {
		copy(e.model.Input(0).F32, e.front.Features(samples))
		err = e.model.Invoke()
	})
	if err != nil {
		return nil, err
	}
	return append([]float32(nil), e.model.Output(0).F32...), nil
}

func Similarity(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range min(len(a), len(b)) {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
