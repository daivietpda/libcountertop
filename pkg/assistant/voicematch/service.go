package voicematch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
	"github.com/ygelfand/libcountertop/pkg/runtime/background"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	helpers     = 1
	nice        = 10
	clips       = 5
	threshold   = 0.5
	unknown     = "Unknown"
	tries       = 3
	waitForWake = 20 * time.Second
)

var ErrNotConfirmed = errors.New(
	"voicematch: the confirmation wake word did not match the new profile",
)

type Stage int

const (
	Prompt Stage = iota + 1
	Heard
	Missed
)

type Options struct {
	Speech   *capture.History
	ModelDir string
	Profiles string
}

const checkPhrase = "voicematch.phrase.lights"

var phrases = [clips + 1]string{
	"voicematch.phrase.door",
	"voicematch.phrase.weather",
	"voicematch.phrase.timer",
	"voicematch.phrase.music",
	"voicematch.phrase.calendar",
	"voicematch.phrase.shopping",
}

type Step struct {
	Number  int
	Of      int
	Confirm bool
	Phrase  string
	Stage   Stage
	Took    time.Duration
}

type Service struct {
	o       Options
	pool    *background.Pool
	matcher *Matcher
	speaker *esphome.TextSensor
	score   *esphome.Sensor

	mu       sync.Mutex
	embed    *voiceprint.Embedder
	settled  bool
	profiles []Profile
	last     []int16
	claims   chan time.Time
}

func (s *Service) Last() []int16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func NewService(o Options) *Service {
	s := &Service{
		o:    o,
		pool: background.New(helpers, nice),
		speaker: &esphome.TextSensor{
			Base: esphome.Base{
				ObjectID: "last_speaker",
				Name:     "Last speaker",
				Icon:     "mdi:account-voice",
			},
		},
		score: &esphome.Sensor{
			Base: esphome.Base{
				ObjectID: "last_speaker_score",
				Name:     "Last speaker score",
				Icon:     "mdi:account-check",
			},
			StateClass: esphome.StateClassMeasurement,
			Decimals:   2,
		},
	}
	s.matcher = New(o.Speech, s, s.heard)
	if raw, err := os.ReadFile(o.Profiles); err == nil {
		if err := json.Unmarshal(raw, &s.profiles); err != nil {
			slog.Warn("voice profiles unreadable", "path", o.Profiles, "err", err)
		}
	}
	return s
}

func (s *Service) Name() string { return "voice match" }

func (s *Service) Entities() []esphome.Entity { return []esphome.Entity{s.speaker, s.score} }

func (s *Service) Actions() []*esphome.Action {
	return []*esphome.Action{{
		Name:        "voice_profiles",
		Description: "Replace the voice profiles this device matches speakers against.",
		Answers:     true,
		Args: []esphome.Arg{{
			Name:        "profiles",
			Type:        esphome.ArgString,
			Description: "JSON list of profiles, each with id, name, person and print.",
		}},
		Run: func(c esphome.Call) (any, error) {
			var ps []Profile
			if err := json.Unmarshal([]byte(c.String("profiles")), &ps); err != nil {
				return nil, fmt.Errorf("voice profiles: %w", err)
			}
			if err := s.SetProfiles(ps); err != nil {
				return nil, err
			}
			return map[string]any{"profiles": len(ps)}, nil
		},
	}}
}

func (s *Service) Profiles() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.profiles)
}

func (s *Service) SetProfiles(ps []Profile) error {
	raw, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.o.Profiles), 0o755); err != nil {
		return fmt.Errorf("voice profiles: %w", err)
	}
	tmp := s.o.Profiles + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("voice profiles: %w", err)
	}
	if err := os.Rename(tmp, s.o.Profiles); err != nil {
		return fmt.Errorf("voice profiles: %w", err)
	}
	s.mu.Lock()
	s.profiles = slices.Clone(ps)
	s.mu.Unlock()
	return nil
}

func (s *Service) Spoke(at time.Time) {
	if len(s.Profiles()) == 0 {
		return
	}
	s.matcher.Spoke(at)
}

func (s *Service) heard(clip []int16, print []float32) {
	s.mu.Lock()
	s.last = clip
	s.mu.Unlock()
	m, ok := best(print, s.Profiles())
	if !ok {
		return
	}
	name := m.Profile.Name
	if m.Score < threshold {
		name = unknown
	}
	slog.Info("voice match", "speaker", name, "closest", m.Profile.Name, "score", m.Score)
	s.speaker.Set(name)
	s.score.Set(float32(m.Score))
}

func (s *Service) Embed(clip []int16) ([]float32, error) {
	e, err := s.embedder(context.Background())
	if err != nil {
		return nil, err
	}
	print, err := e.Embed(clip)
	s.settle()
	return print, err
}

func (s *Service) Recent(ctx context.Context) ([]float32, time.Duration, error) {
	if _, err := s.embedder(ctx); err != nil {
		return nil, 0, err
	}
	start := time.Now()
	print, err := s.Embed(s.o.Speech.Recent(voiceprint.Clip))
	return print, time.Since(start), err
}

func (s *Service) Enroll(
	ctx context.Context,
	id, name, person string,
	prompt func(Step),
) (Profile, Match, error) {
	if _, err := s.embedder(ctx); err != nil {
		return Profile{}, Match{}, err
	}
	claims, release := s.hold()
	defer release()

	prints := make([][]float32, clips)
	errs := make([]error, clips)
	var wg sync.WaitGroup
	for i := range clips {
		clip, err := s.sample(
			ctx,
			claims,
			Step{Number: i + 1, Of: clips, Phrase: say.T(phrases[i])},
			prompt,
		)
		if err != nil {
			wg.Wait()
			return Profile{}, Match{}, err
		}
		wg.Go(func() { prints[i], errs[i] = s.Embed(clip) })
	}
	last, err := s.sample(
		ctx,
		claims,
		Step{Number: clips + 1, Of: clips + 1, Confirm: true, Phrase: say.T(phrases[clips])},
		prompt,
	)
	wg.Wait()
	if err != nil {
		return Profile{}, Match{}, err
	}
	if err := errors.Join(errs...); err != nil {
		return Profile{}, Match{}, err
	}
	profile := Profile{ID: id, Name: name, Person: person, Print: average(prints)}
	print, err := s.Embed(last)
	if err != nil {
		return Profile{}, Match{}, err
	}
	m, ok := confirmed(print, profile, s.Profiles())
	if !ok {
		return profile, m, ErrNotConfirmed
	}
	return profile, m, nil
}

func (s *Service) Check(
	ctx context.Context,
	n int,
	prompt func(Step),
	scored func(Match),
) error {
	if len(s.Profiles()) == 0 {
		return errors.New("voicematch: no profiles to check against")
	}
	if _, err := s.embedder(ctx); err != nil {
		return err
	}
	claims, release := s.hold()
	defer release()
	for i := range n {
		clip, err := s.sample(
			ctx,
			claims,
			Step{Number: i + 1, Of: n, Phrase: say.T(checkPhrase)},
			prompt,
		)
		if err != nil {
			return err
		}
		print, err := s.Embed(clip)
		if err != nil {
			return err
		}
		m, _ := best(print, s.Profiles())
		scored(m)
	}
	return nil
}

func (s *Service) hold() (<-chan time.Time, func()) {
	claims := make(chan time.Time, 1)
	s.mu.Lock()
	s.claims = claims
	s.mu.Unlock()
	return claims, func() {
		s.mu.Lock()
		s.claims = nil
		s.mu.Unlock()
	}
}

func (s *Service) Claim(at time.Time) bool {
	s.mu.Lock()
	claims := s.claims
	s.mu.Unlock()
	if claims == nil {
		return false
	}
	select {
	case claims <- at:
	default:
	}
	return true
}

func (s *Service) sample(
	ctx context.Context,
	claims <-chan time.Time,
	step Step,
	say func(Step),
) ([]int16, error) {
	for range tries {
		select {
		case <-claims:
		default:
		}
		step.Stage = Prompt
		say(step)
		var at time.Time
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case at = <-claims:
		case <-time.After(waitForWake):
			step.Stage = Missed
			say(step)
			continue
		}
		mark := s.o.Speech.MarkBefore(time.Since(at))
		window, err := wakeWindow(ctx, s.o.Speech, at, mark, phraseAfter)
		if err != nil {
			return nil, err
		}
		clip, took, err := wakeClip(window)
		if errors.Is(err, ErrNoSpeech) {
			step.Stage = Missed
			say(step)
			continue
		}
		if err != nil {
			return nil, err
		}
		step.Stage, step.Took = Heard, took
		say(step)
		return clip, nil
	}
	return nil, ErrNoSpeech
}

func confirmed(print []float32, profile Profile, existing []Profile) (Match, bool) {
	others := slices.DeleteFunc(
		slices.Clone(existing),
		func(p Profile) bool { return p.ID == profile.ID },
	)
	m, _ := best(print, append(others, profile))
	return m, m.Profile.ID == profile.ID && m.Score >= threshold
}

func (s *Service) embedder(ctx context.Context) (*voiceprint.Embedder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.embed != nil {
		return s.embed, nil
	}
	model, err := voiceprint.Fetch(ctx, s.o.ModelDir)
	if err != nil {
		return nil, err
	}
	e, err := voiceprint.New(model, s.pool)
	if err != nil {
		return nil, err
	}
	s.embed = e
	return e, nil
}

func (s *Service) settle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.settled {
		s.settled = true
		debug.FreeOSMemory()
	}
}
