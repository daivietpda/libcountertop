package assistant

import (
	"testing"

	voice "github.com/ygelfand/libcountertop/pkg/assistant/turn"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type words schema.Wake

func (w words) Read() schema.Wake { return schema.Wake(w) }

type screen struct{ woke int }

func (s *screen) Wake() { s.woke++ }

func TestATurnWakesTheScreenOnlyForAssistantsThatAsk(t *testing.T) {
	asks := schema.DefaultWakeWord()
	asks.WakeScreen = true
	s := &screen{}
	a := New(Dependencies{
		Words:  words{Words: []schema.WakeWord{schema.DefaultWakeWord(), asks}},
		Screen: s,
	})

	a.Show(voice.Showing{Phase: voice.Listening, Slot: 0})
	a.Show(voice.Showing{Phase: voice.Idle})
	if s.woke != 0 {
		t.Fatalf("assistant 1 woke the screen %d times", s.woke)
	}

	a.Show(voice.Showing{Phase: voice.Listening, Slot: 1})
	a.Show(voice.Showing{Phase: voice.Thinking, Slot: 1})
	a.Show(voice.Showing{Phase: voice.Replying, Slot: 1})
	if s.woke != 1 {
		t.Fatalf("assistant 2 woke the screen %d times in one turn, want once", s.woke)
	}

	a.Show(voice.Showing{Phase: voice.Idle})
	a.Show(voice.Showing{Phase: voice.Listening, Slot: 1})
	if s.woke != 2 {
		t.Errorf("the next turn woke the screen %d times in all, want 2", s.woke)
	}
}
