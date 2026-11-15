package voicematch

import (
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

type clipEmbedder struct{ clips chan []int16 }

func (c clipEmbedder) Embed(clip []int16) ([]float32, error) {
	c.clips <- clip
	return []float32{1}, nil
}

func speech(t *testing.T) []int16 {
	t.Helper()
	raw, err := os.ReadFile("../../audio/vad/testdata/speech.raw")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, len(raw)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return out
}

func TestTheWakeWordAndRequestAreMatched(t *testing.T) {
	h := capture.NewHistory(voiceprint.SampleRate, Span)
	s := speech(t)
	h.Add(make([]int16, voiceprint.SampleRate))
	h.Add(s[voiceprint.SampleRate*6/10 : voiceprint.SampleRate*16/10])

	e := clipEmbedder{clips: make(chan []int16, 1)}
	printed := make(chan []float32, 1)
	New(h, e, func(_ []int16, p []float32) { printed <- p }).Spoke(time.Now())
	h.Add(s[voiceprint.SampleRate*16/10 : voiceprint.SampleRate*36/10])
	h.Add(make([]int16, voiceprint.SampleRate*3/2))

	select {
	case clip := <-e.clips:
		if len(clip) != voiceprint.ClipSamples {
			t.Fatalf("clip has %d samples, want %d", len(clip), voiceprint.ClipSamples)
		}
		var loud int
		for _, s := range clip {
			if s > 1000 || s < -1000 {
				loud++
			}
		}
		if loud < len(clip)/20 {
			t.Errorf("only %d loud samples: the clip should be the speech, repeated", loud)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no clip was embedded")
	}
	if p := <-printed; len(p) != 1 {
		t.Errorf("heard got %v", p)
	}
}
