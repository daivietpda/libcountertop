package level

import (
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	targetDBFS = -23.0
	maxGainDB  = 27.0

	speechOverFloorDB = 12.0

	floorFall = 200 * time.Millisecond
	floorRise = 60 * time.Second

	// Speech peaks 12 to 18 dB above its own average.
	peakDBFS = -3.0

	// The vendor's own leveller carries a fixed 20 dB.
	startGainDB = 20.0

	fallTime = 500 * time.Millisecond
	riseTime = 5 * time.Second
)

const (
	levelLowHz  = 200
	levelHighHz = 900
	levelPoles  = 3

	levelTopDB = 30.0

	levelAttack  = 40 * time.Millisecond
	levelRelease = 180 * time.Millisecond

	overSpan = time.Second
)

// 16-bit rounding error is spread evenly over one step; its average size is 1/√12 of a step.
const quietest = 0.28867513459481287

const fullScale = 32768

type Config struct {
	Rate  int
	Frame int

	Sensitivity int
	Leveling    bool
}

type Leveler struct {
	gain float32

	quiet atomic.Int32

	sounding bool

	leveling    atomic.Bool
	wasLeveling bool

	peak atomic.Uint32

	floor float32

	fall, rise          float32
	floorDown, floorUp  float32
	target, ceiling     float32
	headroom, overFloor float32

	levelUp, levelDown float32

	lp                 [levelPoles]float32
	hp                 float32
	lpA, hpA           float32
	bandRMS, bandFloor float32

	published  atomic.Uint32
	shownFloor atomic.Uint32
	level      atomic.Uint32
	clipped    atomic.Uint64

	frame   time.Duration
	overMu  sync.Mutex
	overs   []float32
	overAt  int
	overLen int
}

func New(c Config) *Leveler {
	frame := float64(c.Frame) / float64(c.Rate)
	rate := float64(c.Rate)

	l := &Leveler{
		gain:      float32(math.Pow(10, startGainDB/20)),
		floor:     fullScale,
		fall:      float32(1 - math.Exp(-frame/fallTime.Seconds())),
		rise:      float32(1 - math.Exp(-frame/riseTime.Seconds())),
		floorDown: float32(1 - math.Exp(-frame/floorFall.Seconds())),
		floorUp:   float32(1 - math.Exp(-frame/floorRise.Seconds())),
		target:    float32(math.Pow(10, targetDBFS/20) * fullScale),
		ceiling:   float32(math.Pow(10, maxGainDB/20)),
		headroom:  float32(math.Pow(10, peakDBFS/20) * fullScale),
		overFloor: float32(math.Pow(10, speechOverFloorDB/20)),

		levelUp:   float32(1 - math.Exp(-frame/levelAttack.Seconds())),
		levelDown: float32(1 - math.Exp(-frame/levelRelease.Seconds())),

		bandFloor: fullScale,
		lpA:       float32(1 - math.Exp(-2*math.Pi*levelHighHz/rate)),
		hpA:       float32(1 - math.Exp(-2*math.Pi*levelLowHz/rate)),

		frame: time.Duration(frame * float64(time.Second)),
		overs: make([]float32, max(int(overSpan.Seconds()*rate)/max(c.Frame, 1), 1)),
	}

	l.quiet.Store(int32(c.Sensitivity))
	l.leveling.Store(c.Leveling)
	l.wasLeveling = c.Leveling
	l.publish()
	return l
}

func (l *Leveler) SetSensitivity(db int) {
	l.quiet.Store(int32(db))
	slog.Info("room sensitivity", "over_floor_db", db)
}

func (l *Leveler) SetLeveling(on bool) {
	l.leveling.Store(on)
	slog.Info("microphone leveling", "on", on)
}

func (l *Leveler) SetPlayback(on bool) { l.sounding = on }

func (l *Leveler) Process(heard, frame []int16) {
	on := l.leveling.Load()
	switch {
	case on:
		l.apply(heard, frame)
	default:
		if l.wasLeveling {
			l.forget()
		}
		if len(heard) > 0 {
			l.observe(heard)
		}
	}
	l.wasLeveling = on
}

func (l *Leveler) Floor() float64 {
	return 20 * math.Log10(float64(math.Float32frombits(l.shownFloor.Load()))/fullScale)
}

func (l *Leveler) Peak() float64 {
	loudest := max(math.Float32frombits(l.peak.Swap(0)), quietest)
	return 20 * math.Log10(float64(loudest)/fullScale)
}

func (l *Leveler) Gain() float64 {
	return 20 * math.Log10(float64(math.Float32frombits(l.published.Load())))
}

func (l *Leveler) Clipped() uint64 { return l.clipped.Load() }

func (l *Leveler) Level() float64 {
	return float64(math.Float32frombits(l.level.Load()))
}

func (l *Leveler) forget() {
	l.gain = float32(math.Pow(10, startGainDB/20))
	l.floor, l.bandFloor = fullScale, fullScale
	l.publish()
}

func (l *Leveler) publish() {
	l.published.Store(math.Float32bits(l.gain))
	l.shownFloor.Store(math.Float32bits(l.bandFloor))
}

func abs(s int16) int32 {
	if s < 0 {
		return -int32(s)
	}
	return int32(s)
}

func (l *Leveler) observe(frame []int16) (rms float32) {
	var sum, band float64
	for _, s := range frame {
		sum += float64(s) * float64(s)

		v := float32(s)
		for i := range l.lp {
			l.lp[i] += (v - l.lp[i]) * l.lpA
			v = l.lp[i]
		}
		l.hp += (v - l.hp) * l.hpA
		v -= l.hp

		band += float64(v) * float64(v)
	}
	rms = float32(math.Sqrt(sum / float64(len(frame))))
	l.bandRMS = float32(math.Sqrt(band / float64(len(frame))))

	if !l.sounding {
		l.floor = follow(l.floor, rms, l.floorDown, l.floorUp)
		l.bandFloor = follow(l.bandFloor, l.bandRMS, l.floorDown, l.floorUp)
		l.shownFloor.Store(math.Float32bits(l.bandFloor))

		if l.bandRMS > math.Float32frombits(l.peak.Load()) {
			l.peak.Store(math.Float32bits(l.bandRMS))
		}
	}

	l.publishLevel(l.bandRMS)
	return rms
}

func loudest(frame []int16) float32 {
	var peak int32
	for _, s := range frame {
		peak = max(peak, abs(s))
	}
	return float32(peak)
}

func follow(floor, rms, down, up float32) float32 {
	step := up
	if rms < floor {
		step = down
	}
	return max(floor+(rms-floor)*step, quietest)
}

func levelFrom(overDB, quiet, span float64) float64 {
	x := min(max((overDB-quiet)/span, 0), 1)

	return x * x * x * (x*(6*x-15) + 10)
}

func (l *Leveler) Over(d time.Duration) float64 {
	l.overMu.Lock()
	defer l.overMu.Unlock()
	n := l.overLen
	if l.frame > 0 {
		n = min(n, int(d/l.frame))
	}
	var loudest float32
	for i := range n {
		loudest = max(loudest, l.overs[(l.overAt-1-i+len(l.overs))%len(l.overs)])
	}
	return float64(loudest)
}

func (l *Leveler) remember(over float64) {
	l.overMu.Lock()
	defer l.overMu.Unlock()
	l.overs[l.overAt] = float32(over)
	l.overAt = (l.overAt + 1) % len(l.overs)
	l.overLen = min(l.overLen+1, len(l.overs))
}

func (l *Leveler) publishLevel(rms float32) {
	over := 0.0
	if rms > l.bandFloor {
		over = 20 * math.Log10(float64(rms/l.bandFloor))
	}
	l.remember(over)
	quiet := float64(l.quiet.Load())
	want := float32(levelFrom(over, quiet, levelTopDB-quiet))

	was := math.Float32frombits(l.level.Load())
	step := l.levelUp
	if want < was {
		step = l.levelDown
	}
	l.level.Store(math.Float32bits(was + (want-was)*step))
}

func (l *Leveler) apply(heard, frame []int16) {
	if len(frame) == 0 {
		return
	}
	rms := l.observe(heard)
	peak := loudest(frame)

	if !l.sounding && rms > l.floor*l.overFloor {
		want := min(max(l.target/rms, 1), l.ceiling)

		step := l.rise
		if want < l.gain {
			step = l.fall
		}
		l.gain += (want - l.gain) * step
	}

	gain := l.gain
	if peak > 0 {
		gain = min(gain, l.headroom/peak)
	}

	l.publish()

	for i, s := range frame {
		switch v := float32(s) * gain; {
		case v > fullScale-1:
			frame[i] = fullScale - 1
			l.clipped.Add(1)
		case v < -fullScale:
			frame[i] = -fullScale
			l.clipped.Add(1)
		default:
			frame[i] = int16(v)
		}
	}
}
