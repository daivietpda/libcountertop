package turn

import (
	"context"
	"time"

	"github.com/ygelfand/libcountertop/pkg/assistant/activity"
	"github.com/ygelfand/libcountertop/pkg/inference/wake"
)

type Microphone interface {
	Listen(string) (<-chan []int16, func())
	Recent(time.Duration) []int16
	Over(time.Duration) float64
	Floor() float64
	Gain() float64
	SetAdapting(bool)
	Clipped() uint64
}

type Speaker interface {
	VoiceSink
	Splices() uint64
	Underruns() uint64
	Clipped() uint64
}

type Claim interface {
	Done() <-chan struct{}
	Err() error
	Stopped() bool
	Quiet() time.Time
}

type Sound interface {
	Claim(string, func(context.Context, Speaker) error) Claim
	Busy() bool
	Silence()
	Duck(bool)
}

type Arbiter interface {
	Decide(word string, score float64) bool
}

type Voices interface {
	Spoke(at time.Time)
	Claim(at time.Time) bool
}

type Recorder interface {
	Opens(string, int)
	Frame([]byte)
	Closes()
	Seconds(string) float64
}

type Dependencies struct {
	Source             Microphone
	Speaker            Speaker
	Sound              Sound
	Player             interface{ Sounding(bool) }
	Log                *activity.Log
	Recorder           Recorder
	Words              Words
	Word               func(int) (string, bool)
	Models             func() []wake.Model
	Muted              func() (bool, error)
	HardwareTail       time.Duration
	Presentation       *Presentation
	Arbiter            Arbiter
	Voices             Voices
	Cancelled, Failure func()
}

func dropped(source Microphone) uint64 {
	if s, ok := source.(interface{ Dropped() uint64 }); ok {
		return s.Dropped()
	}
	return 0
}
