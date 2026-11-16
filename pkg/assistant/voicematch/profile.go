package voicematch

import (
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

type Profile struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Person string      `json:"person,omitempty"`
	Prints [][]float32 `json:"prints"`
}

type Match struct {
	Profile Profile
	Score   float64
}

func best(print []float32, profiles []Profile) (Match, bool) {
	var m Match
	found := false
	for _, p := range profiles {
		for _, take := range p.Prints {
			if s := voiceprint.Similarity(print, take); !found || s > m.Score {
				m, found = Match{Profile: p, Score: s}, true
			}
		}
	}
	return m, found
}
