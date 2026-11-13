package wakesettings

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/assistant/recordings"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const Domain = "assistants"

func (s *Settings) Controls() *setting.Controls[schema.Wake] {
	return &setting.Controls[schema.Wake]{
		Table: s.table(),
		Read:  s.options.Read,
		Save:  s.save,
	}
}

func (s *Settings) table() *setting.Table[schema.Wake] {
	var (
		groups []setting.Group
		rows   []setting.Setting[schema.Wake]
	)
	for n := range s.options.Slots {
		groups = append(groups, SlotGroup(n))
		rows = append(rows, s.rows(n)...)
	}
	return setting.NewTable(Domain, groups, rows, SlotMessages(Domain))
}

func (s *Settings) rows(n int) []setting.Setting[schema.Wake] {
	row := func(field string) setting.Setting[schema.Wake] {
		return setting.Setting[schema.Wake]{
			Name:     SlotName(n, field),
			Group:    SlotGroup(n),
			NoEntity: true,
		}
	}
	word := func(c *schema.Wake) *schema.WakeWord { return wordAt(c, n) }

	sensitivity := row("threshold")
	sensitivity.Kind = setting.Number
	sensitivity.Min, sensitivity.Max, sensitivity.Unit = 50, 99, "%"
	sensitivity.Read = func(c *schema.Wake) string {
		return strconv.Itoa(int(math.Round(word(c).Threshold * 100)))
	}
	sensitivity.Write = func(c *schema.Wake, v string) error {
		p, err := sensitivity.Number(v)
		if err != nil {
			return err
		}
		word(c).Threshold = float64(p) / 100
		return nil
	}

	tone := row("tone")
	tone.More = s.tones

	count := func(field string, least, most int, unit string, value func(*schema.WakeWord) *int) setting.Setting[schema.Wake] {
		r := row(field)
		r.Min, r.Max, r.Unit = least, most, unit
		return setting.Count(r, func(c *schema.Wake) *int { return value(word(c)) })
	}

	rows := []setting.Setting[schema.Wake]{
		sensitivity,
		setting.Pick(
			tone,
			schema.Chimes(),
			func(c *schema.Wake) *schema.Chime { return &word(c).Tone },
		),
		setting.Pick(
			row("delivery"),
			schema.Deliveries(),
			func(c *schema.Wake) *schema.Delivery { return &word(c).Delivery },
		),
		count("follow_up", 0, 30, "s", func(w *schema.WakeWord) *int { return &w.FollowUp }),
		count("buffer", 0, 3000, "ms", func(w *schema.WakeWord) *int { return &w.Buffer }),
		count("max_listen", 5, 60, "s", func(w *schema.WakeWord) *int { return &w.MaxListen }),
		count("max_think", 5, 300, "s", func(w *schema.WakeWord) *int { return &w.MaxThink }),
		count(
			"recordings",
			0,
			recordings.KeepMost,
			"",
			func(w *schema.WakeWord) *int { return &w.Recordings },
		),
	}
	if s.options.Screen {
		rows = append(rows, setting.Flag(
			row("wake_screen"),
			func(c *schema.Wake) *bool { return &word(c).WakeScreen },
		))
	}
	return rows
}

func (s *Settings) save(row setting.Setting[schema.Wake], value string) error {
	n, err := SlotOf(row.Name)
	if err != nil {
		return err
	}
	var bad error
	if err := s.options.Write(n).Apply(func(w *schema.WakeWord) {
		probe := schema.Wake{Words: make([]schema.WakeWord, n+1)}
		probe.Words[n] = *w
		if bad = row.Write(&probe, value); bad == nil {
			*w = probe.Words[n]
		}
	}); err != nil {
		return err
	}
	if bad != nil {
		return bad
	}
	s.Restore(s.options.Read().Slots(len(s.slots)))
	return nil
}

func wordAt(c *schema.Wake, n int) *schema.WakeWord {
	words := slices.Clone(c.Words)
	for len(words) <= n {
		words = append(words, schema.DefaultWakeWord())
	}
	c.Words = words
	return &c.Words[n]
}

func SlotGroup(n int) setting.Group { return setting.Group(fmt.Sprintf("Assistant %d", n+1)) }

func SlotName(n int, field string) string { return fmt.Sprintf("%d.%s", n+1, field) }

func SlotOf(name string) (int, error) {
	at, _, _ := strings.Cut(name, ".")
	n, err := strconv.Atoi(at)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("no assistant in %q", name)
	}
	return n - 1, nil
}

func SlotMessages(domain string) setting.Messages {
	unslotted := func(key string) string {
		rest, ok := strings.CutPrefix(key, domain+".setting.")
		if !ok {
			return key
		}
		_, field, found := strings.Cut(rest, ".")
		if !found {
			return key
		}
		return domain + ".setting." + field
	}
	return setting.Messages{
		Text: func(key string) string {
			if n, ok := strings.CutPrefix(key, domain+".group.assistant"); ok {
				return say.F(Domain+".slot", map[string]any{"N": n})
			}
			return say.T(unslotted(key))
		},
		Missing: func(key string) bool { return say.Missing(unslotted(key)) },
	}
}
