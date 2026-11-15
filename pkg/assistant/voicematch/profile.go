package voicematch

import (
	"math"

	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

type Profile struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Person string    `json:"person,omitempty"`
	Print  []float32 `json:"print"`
}

type Match struct {
	Profile Profile
	Score   float64
}

func best(print []float32, profiles []Profile) (Match, bool) {
	var m Match
	found := false
	for _, p := range profiles {
		if s := voiceprint.Similarity(print, p.Print); !found || s > m.Score {
			m, found = Match{Profile: p, Score: s}, true
		}
	}
	return m, found
}

func unit(v []float32) []float32 {
	var sq float64
	for _, x := range v {
		sq += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if sq == 0 {
		return out
	}
	inv := 1 / math.Sqrt(sq)
	for i, x := range v {
		out[i] = float32(float64(x) * inv)
	}
	return out
}

func average(prints [][]float32) []float32 {
	if len(prints) == 0 {
		return nil
	}
	sum := make([]float32, len(prints[0]))
	for _, p := range prints {
		for i, x := range unit(p) {
			sum[i] += x
		}
	}
	return unit(sum)
}
