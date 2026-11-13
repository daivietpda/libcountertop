package assistant

import (
	"context"
	"log/slog"
	"sync"
	"time"

	voice "github.com/ygelfand/libcountertop/pkg/assistant/turn"
	"github.com/ygelfand/libcountertop/pkg/hook"
)

const (
	frame = 50 * time.Millisecond

	slide = 220 * time.Millisecond

	// Ordinary speech is around fifteen characters a second.
	reading = 15.0
)

type Assistant struct {
	options Dependencies
	Frame   hook.Hook[Showing]

	mu sync.Mutex

	turn  voice.Showing
	since time.Time

	opened  time.Time
	closing time.Time

	wake chan struct{}
	took *int
}

type Level interface{ Level() float64 }
type Controller interface{ Stop() bool }
type Waker interface{ Wake() }
type Dependencies struct {
	Microphone Level
	Controller Controller
	Words      WakeSettings
	Screen     Waker
}

func New(options Dependencies) *Assistant {
	return &Assistant{options: options, wake: make(chan struct{}, 1)}
}

func (a *Assistant) Name() string { return "assistant screen" }

func (a *Assistant) Show(s voice.Showing) {
	a.mu.Lock()

	if s.Reply != a.turn.Reply || a.turn.Phase == voice.Idle {
		a.since = time.Now()
	}
	began := a.turn.Phase == voice.Idle && s.Phase != voice.Idle
	if began {
		a.opened, a.closing = time.Now(), time.Time{}
	}
	if s.Phase == voice.Idle && a.turn.Phase != voice.Idle {
		a.closing = time.Now()
	}
	a.turn = s
	a.mu.Unlock()

	if began {
		a.wakeScreen(s.Slot)
	}

	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *Assistant) wakeScreen(slot int) {
	o := a.options
	if o.Words == nil || o.Screen == nil || !o.Words.Read().Slot(slot).WakeScreen {
		return
	}
	o.Screen.Wake()
}

func (a *Assistant) Run(ctx context.Context) error {
	for {
		if !a.showing() {
			a.Hide()

			select {
			case <-ctx.Done():
				return nil
			case <-a.wake:
			}
			continue
		}

		a.Paint()

		select {
		case <-ctx.Done():
			return nil
		case <-a.wake:
		case <-time.After(frame):
		}
	}
}

func (a *Assistant) showing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.turn.Phase != voice.Idle {
		return true
	}
	return !a.closing.IsZero() && time.Since(a.closing) < slide
}

func (a *Assistant) Now() (Showing, bool) {
	now := time.Now()

	a.mu.Lock()
	show := Showing{
		Showing: a.turn,
		At:      now,
		Down:    a.down(now),
		Reveal:  a.reveal(now),
	}
	a.mu.Unlock()

	if show.Phase == voice.Listening {
		show.Level = a.options.Microphone.Level()
	}
	return show, show.Down > 0
}

func (a *Assistant) Paint() {
	show, _ := a.Now()
	a.Frame.Emit(show)
}

func (a *Assistant) down(now time.Time) float64 {
	if a.turn.Phase == voice.Idle {
		if a.closing.IsZero() {
			return 0
		}
		return 1 - min(now.Sub(a.closing).Seconds()/slide.Seconds(), 1)
	}
	if a.opened.IsZero() {
		return 1
	}
	return min(now.Sub(a.opened).Seconds()/slide.Seconds(), 1)
}

func (a *Assistant) reveal(now time.Time) float64 {
	if a.turn.Reply == "" || a.turn.Phase != voice.Replying {
		return 0
	}

	runes := float64(len([]rune(a.turn.Reply)))
	if runes == 0 {
		return 1
	}
	return min(now.Sub(a.since).Seconds()*reading/runes, 1)
}

func (a *Assistant) Hide() { a.Frame.Emit(Showing{}) }

func (a *Assistant) Took(id int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.took != nil && *a.took == id
}

func (a *Assistant) Cancel(id int) {
	a.mu.Lock()
	showing := a.turn.Phase != voice.Idle
	a.took = nil
	if showing {
		a.took = &id
	}
	a.mu.Unlock()

	if !showing {
		return
	}
	if a.options.Controller.Stop() {
		slog.Info("turn ended by touch")
	}
}
