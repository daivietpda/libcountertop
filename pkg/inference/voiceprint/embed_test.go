package voiceprint

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type inline struct{}

func (inline) Split(n int, part func(lo, hi int)) { part(0, n) }
func (inline) Run(f func())                       { f() }

func TestModelMatchesTheReference(t *testing.T) {
	model, ref := os.Getenv("VOICEPRINT_MODEL"), os.Getenv("VOICEPRINT_REFERENCE")
	if model == "" || ref == "" {
		t.Skip("set VOICEPRINT_MODEL and VOICEPRINT_REFERENCE to check a converted model")
	}
	raw, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(raw, inline{})
	if err != nil {
		t.Fatal(err)
	}

	pcm, err := os.ReadFile(filepath.Join(ref, "clip.s16"))
	if err != nil {
		t.Fatal(err)
	}
	clip := make([]int16, len(pcm)/2)
	for i := range clip {
		clip[i] = int16(binary.LittleEndian.Uint16(pcm[2*i:]))
	}
	emb, err := os.ReadFile(filepath.Join(ref, "clip_emb.f32"))
	if err != nil {
		t.Fatal(err)
	}
	want := make([]float32, len(emb)/4)
	for i := range want {
		want[i] = math.Float32frombits(binary.LittleEndian.Uint32(emb[4*i:]))
	}

	got, err := e.Embed(clip)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("voiceprint has %d values, reference %d", len(got), len(want))
	}
	if s := Similarity(got, want); s < 0.99999 {
		t.Errorf("similarity to the reference is %.7f, want at least 0.99999", s)
	}
}

func TestSimilarity(t *testing.T) {
	a := []float32{1, 2, 3}
	if s := Similarity(a, []float32{2, 4, 6}); math.Abs(s-1) > 1e-9 {
		t.Errorf("parallel vectors: %v, want 1", s)
	}
	if s := Similarity(a, []float32{-1, -2, -3}); math.Abs(s+1) > 1e-9 {
		t.Errorf("opposite vectors: %v, want -1", s)
	}
	if s := Similarity(a, []float32{0, 0, 0}); s != 0 {
		t.Errorf("against zero: %v, want 0", s)
	}
}

func TestEmbedRefusesAClipOfTheWrongLength(t *testing.T) {
	e := &Embedder{run: inline{}}
	if _, err := e.Embed(make([]int16, 100)); err == nil {
		t.Error("a 100-sample clip should be refused")
	}
}
