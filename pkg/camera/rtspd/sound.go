package rtspd

import (
	"log/slog"
	"sync"
	"time"

	aac "github.com/tphakala/go-aac"

	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	"github.com/ygelfand/libcountertop/pkg/audio/upsample"
	"github.com/ygelfand/libcountertop/pkg/media/rtsp"
)

const (
	soundRate    = 48000
	soundBitrate = 48000
	soundCutoff  = upsample.VoiceRate / 2
)

type Mic interface {
	Frames() (<-chan []int16, func())
}

type sound struct {
	mic     Mic
	muted   func() bool
	streams []*rtsp.Stream
	format  rtsp.AAC

	mu      sync.Mutex
	running bool
	wanted  time.Time
	stop    chan struct{}
}

func soundConfig() aac.EncoderConfig {
	return aac.EncoderConfig{
		SampleRate: soundRate,
		Channels:   1,
		Bitrate:    soundBitrate,
		Cutoff:     soundCutoff,
	}
}

func newSound(mic Mic, muted func() bool) (*sound, error) {
	enc, err := aac.NewEncoder(soundConfig())
	if err != nil {
		return nil, err
	}
	return &sound{
		mic:    mic,
		muted:  muted,
		format: rtsp.AAC{Rate: soundRate, Channels: 1, Config: enc.AudioSpecificConfig()},
	}, nil
}

func (a *sound) carry(st *rtsp.Stream) {
	format := a.format
	st.Audio = &format
	a.streams = append(a.streams, st)
}

func (a *sound) demand() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.wanted = time.Now()
	if a.running {
		return
	}
	a.running = true
	a.stop = make(chan struct{})
	go a.run(a.stop)
}

func (a *sound) halt() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running && a.stop != nil {
		close(a.stop)
		a.stop = nil
	}
}

func (a *sound) idle() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.wanted) <= linger {
		return false
	}
	for _, st := range a.streams {
		if st.Viewers() > 0 {
			return false
		}
	}
	return true
}

func (a *sound) run(stop chan struct{}) {
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	enc, err := aac.NewEncoder(soundConfig())
	if err != nil {
		slog.Warn("the rtsp audio encoder failed", "err", err)
		return
	}
	frames, done := a.mic.Frames()
	defer done()
	up := resample.NewRational(upsample.VoiceRate, soundRate, 1)
	pending := make([]float32, 0, 2*aac.FrameSize)
	var au []byte
	var sent int64
	slog.Info("rtsp audio up", "rate", soundRate, "bitrate", soundBitrate)
	for {
		select {
		case <-stop:
			return
		case in, ok := <-frames:
			if !ok {
				return
			}
			if a.idle() {
				slog.Info("rtsp audio down")
				return
			}
			silent := a.muted != nil && a.muted()
			for _, v := range up.Run(in) {
				if silent {
					v = 0
				}
				pending = append(pending, float32(v)/(1<<15))
			}
			for len(pending) >= aac.FrameSize {
				au, err = enc.EncodeFrame(au[:0], [][]float32{pending[:aac.FrameSize]})
				pending = append(pending[:0], pending[aac.FrameSize:]...)
				if err != nil {
					slog.Warn("the rtsp audio encoder failed", "err", err)
					return
				}
				if len(au) == 0 {
					continue
				}
				pts := time.Duration(sent) * time.Second / soundRate
				sent += aac.FrameSize
				for _, st := range a.streams {
					st.WriteAudio(au, pts)
				}
			}
		}
	}
}
