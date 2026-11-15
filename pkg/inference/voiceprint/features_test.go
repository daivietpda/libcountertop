package voiceprint

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func testSignal(n int) []float32 {
	s := uint64(1)
	out := make([]float32, n)
	for i := range out {
		s = (s*1103515245 + 12345) % (1 << 31)
		noise := float64(s)/float64(1<<31) - 0.5
		out[i] = float32(0.3*math.Sin(2*math.Pi*220*float64(i)/SampleRate) +
			0.2*math.Sin(2*math.Pi*1375.5*float64(i)/SampleRate) +
			0.05*noise)
	}
	return out
}

func TestFeaturesMatchTheReferenceFrontend(t *testing.T) {
	raw, err := os.ReadFile("testdata/mel_short.f32")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]float32, len(raw)/4)
	for i := range want {
		want[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}

	samples := testSignal(SampleRate)
	got := NewFrontend().Features(samples)
	if frames := Frames(len(samples)); frames != 99 || len(got) != len(want) {
		t.Fatalf("got %d values over %d frames, want %d over 99", len(got), frames, len(want))
	}
	var worst float64
	for i := range want {
		worst = max(worst, math.Abs(float64(got[i]-want[i])))
	}
	if worst > 5e-4 {
		t.Errorf("largest difference from the reference is %.5f", worst)
	}
}

func TestFramesForAThreeSecondClip(t *testing.T) {
	if got := Frames(48080); got != 300 {
		t.Errorf("Frames(48080) = %d, want 300", got)
	}
	if got := Frames(100); got != 0 {
		t.Errorf("Frames(100) = %d, want 0", got)
	}
}
