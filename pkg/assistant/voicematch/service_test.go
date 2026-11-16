package voicematch

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

func newService(t *testing.T) *Service {
	t.Helper()
	return NewService(Options{
		Speech:   capture.NewHistory(voiceprint.SampleRate, Span),
		ModelDir: t.TempDir(),
		Profiles: filepath.Join(t.TempDir(), "voices.json"),
	})
}

func TestTheClosestTakeDecides(t *testing.T) {
	ann := Profile{ID: "a", Name: "Ann", Prints: [][]float32{{1, 0}, {0, 1}}}
	bo := Profile{ID: "b", Name: "Bo", Prints: [][]float32{{0.7, 0.7}}}
	m, ok := best([]float32{0, 1}, []Profile{bo, ann})
	if !ok || m.Profile.ID != "a" || m.Score < 0.99 {
		t.Errorf("a clip matching one of Ann's takes gave %+v %v", m, ok)
	}
}

func TestProfilesSurviveARestart(t *testing.T) {
	s := newService(t)
	want := []Profile{
		{ID: "a", Name: "Ann", Person: "person.ann", Prints: [][]float32{{1, 0}, {0, 1}}},
	}
	if err := s.SetProfiles(want); err != nil {
		t.Fatal(err)
	}
	again := NewService(s.o)
	if got := again.Profiles(); !reflect.DeepEqual(got, want) {
		t.Errorf("after reloading: %+v", got)
	}
}

func TestProfilesActionReplacesTheSet(t *testing.T) {
	s := newService(t)
	_ = s.SetProfiles([]Profile{{ID: "old", Name: "Old", Prints: [][]float32{{1}}}})
	action := s.Actions()[0]
	call, err := esphome.NewCall(map[string]any{
		"profiles": `[{"id":"a","name":"Ann","prints":[[1,0]]},{"id":"b","name":"Bo","prints":[[0,1]]}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := action.Run(call); err != nil {
		t.Fatal(err)
	}
	if got := s.Profiles(); len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("profiles after the action: %+v", got)
	}
	bad, err := esphome.NewCall(map[string]any{"profiles": "not json"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := action.Run(bad); err == nil {
		t.Error("malformed profiles should be refused")
	}
}

func TestLastSpeakerNamesTheClosestAboveTheThreshold(t *testing.T) {
	s := newService(t)
	_ = s.SetProfiles([]Profile{
		{ID: "a", Name: "Ann", Prints: [][]float32{{1, 0}}},
		{ID: "b", Name: "Bo", Prints: [][]float32{{0, 1}}},
	})
	s.heard([]int16{7}, []float32{0.1, 1})
	if got := s.speaker.Get(); got != "Bo" {
		t.Errorf("speaker %q, want Bo", got)
	}
	if got := s.Last(); len(got) != 1 || got[0] != 7 {
		t.Errorf("last clip %v, want the one just matched", got)
	}
	s.heard(nil, []float32{1, 1})
	if got := s.speaker.Get(); got != "Ann" && got != "Bo" {
		t.Errorf("an even split above the threshold should still name someone, got %q", got)
	}
	s.heard(nil, []float32{-1, -1})
	if got := s.speaker.Get(); got != unknown {
		t.Errorf("speaker %q, want %q", got, unknown)
	}
}

func TestWakeClipKeepsOnlyTheSpeechAndRepeatsIt(t *testing.T) {
	voice := speech(t)[voiceprint.SampleRate*6/10 : voiceprint.SampleRate*16/10]
	window := slices.Concat(
		make([]int16, voiceprint.SampleRate/2),
		voice,
		make([]int16, voiceprint.SampleRate*3/10),
	)
	clip, took, err := wakeClip(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip) != voiceprint.ClipSamples {
		t.Fatalf("clip has %d samples, want %d", len(clip), voiceprint.ClipSamples)
	}
	if took < 600*time.Millisecond || took > 1300*time.Millisecond {
		t.Errorf("kept %v of speech from a window with 1 s of it", took)
	}
	kept := int(took * voiceprint.SampleRate / time.Second)
	if !slices.Equal(clip[:kept], clip[kept:2*kept]) {
		t.Error("the speech should repeat to fill the clip")
	}
}

func TestWakeClipRefusesSilence(t *testing.T) {
	if _, _, err := wakeClip(make([]int16, voiceprint.SampleRate*2)); !errors.Is(err, ErrNoSpeech) {
		t.Errorf("silence gave %v, want ErrNoSpeech", err)
	}
}

func TestClaimOnlyWhileEnrolling(t *testing.T) {
	s := newService(t)
	if s.Claim(time.Now()) {
		t.Error("a wake word should start a turn when nobody is enrolling")
	}
	s.claims = make(chan time.Time, 1)
	if !s.Claim(time.Now()) || !s.Claim(time.Now()) {
		t.Error("while enrolling every wake word belongs to the enrolment")
	}
}

func TestConfirmationNeedsTheNewProfileToWin(t *testing.T) {
	ann := Profile{ID: "a", Name: "Ann", Prints: [][]float32{{1, 0}}}
	bo := Profile{ID: "b", Name: "Bo", Prints: [][]float32{{0, 1}}}
	if _, ok := confirmed([]float32{1, 0.1}, ann, []Profile{bo}); !ok {
		t.Error("a clip close to the new profile should confirm")
	}
	if m, ok := confirmed([]float32{0.1, 1}, ann, []Profile{bo}); ok || m.Profile.ID != "b" {
		t.Errorf("a clip closer to another profile should not confirm: %+v %v", m, ok)
	}
	if _, ok := confirmed([]float32{-1, 0}, ann, nil); ok {
		t.Error("a clip below the threshold should not confirm")
	}
	if _, ok := confirmed(
		[]float32{1, 0},
		ann,
		[]Profile{{ID: "a", Name: "Ann", Prints: [][]float32{{0, 1}}}},
	); !ok {
		t.Error("re-enrolling should compare against the new profile, not the old copy")
	}
}
