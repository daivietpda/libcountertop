package wakesettings

import (
	"fmt"
	"log/slog"

	esphome "github.com/ygelfand/go-esphome-device"

	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type Options struct {
	Slots    int
	Screen   bool
	DeviceID func(slot int) uint32
	Read     func() schema.Wake
	Write    func(slot int) schema.WakeWriter
	Wake     func(slot int)
	Extras   func(slot int) []esphome.Entity
	Tones    func() []setting.Option
}

type Settings struct {
	options Options
	slots   []slot
}

type slot struct {
	wake *esphome.Button

	threshold  *esphome.Number
	tone       *esphome.Select
	delivery   *esphome.Select
	buffer     *esphome.Number
	followUp   *esphome.Number
	maxListen  *esphome.Number
	maxThink   *esphome.Number
	wakeScreen *esphome.Switch
}

func New(options Options) *Settings {
	s := &Settings{options: options}
	for n := range options.Slots {
		s.slots = append(s.slots, s.newSlot(n))
	}
	return s
}

func (s *Settings) newSlot(n int) slot {
	write := func() schema.WakeWriter { return s.options.Write(n) }

	sl := slot{
		wake: &esphome.Button{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("wake_assistant_%d", n+1),
				Name:     "Wake",
				Icon:     "mdi:account-voice",
			},
			OnPress: func() {
				if s.options.Wake != nil {
					s.options.Wake(n)
				}
			},
		},
		threshold: &esphome.Number{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("wake_threshold_%d", n+1),
				Name:     "Wake word sensitivity",
				Icon:     "mdi:tune",
				Category: esphome.CategoryConfig,
			},
			Min: 0.5, Max: 0.99, Step: 0.01,
			Mode: esphome.NumberBox,
		},
		tone: &esphome.Select{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("wake_tone_%d", n+1),
				Name:     "Wake word tone",
				Icon:     "mdi:music-note",
				Category: esphome.CategoryConfig,
			},
			Options: s.toneLabels(),
		},
		delivery: &esphome.Select{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("reply_delivery_%d", n+1),
				Name:     "Reply delivery",
				Icon:     "mdi:download-network",
				Category: esphome.CategoryConfig,
			},
			Options: labels(schema.Deliveries()),
		},
		buffer: &esphome.Number{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("reply_buffer_%d", n+1),
				Name:     "Reply buffer",
				Icon:     "mdi:buffer",
				Category: esphome.CategoryConfig,
			},
			Min: 0, Max: 3000, Step: 50, Unit: "ms",
			Mode: esphome.NumberBox,
		},
		followUp: &esphome.Number{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("follow_up_%d", n+1),
				Name:     "Follow-up time",
				Icon:     "mdi:comment-question-outline",
				Category: esphome.CategoryConfig,
			},
			Min: 0, Max: 30, Step: 1, Unit: "s",
			Mode: esphome.NumberBox,
		},
		maxListen: &esphome.Number{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("max_listen_%d", n+1),
				Name:     "Max listening time",
				Icon:     "mdi:timer-outline",
				Category: esphome.CategoryConfig,
			},
			Min: 5, Max: 60, Step: 1, Unit: "s",
			Mode: esphome.NumberBox,
		},
		maxThink: &esphome.Number{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("max_think_%d", n+1),
				Name:     "Max thinking time",
				Icon:     "mdi:timer-sand",
				Category: esphome.CategoryConfig,
			},
			Min: 5, Max: 300, Step: 5, Unit: "s",
			Mode: esphome.NumberBox,
		},
		wakeScreen: &esphome.Switch{
			Base: esphome.Base{
				ObjectID: fmt.Sprintf("wake_screen_%d", n+1),
				Name:     "Wake the screen",
				Icon:     "mdi:monitor-eye",
				Category: esphome.CategoryConfig,
			},
		},
	}

	if s.options.DeviceID != nil {
		on := s.options.DeviceID(n)
		for _, b := range []*esphome.Base{
			&sl.wake.Base, &sl.threshold.Base, &sl.tone.Base, &sl.delivery.Base,
			&sl.buffer.Base, &sl.followUp.Base, &sl.maxListen.Base, &sl.maxThink.Base,
			&sl.wakeScreen.Base,
		} {
			b.DeviceID = on
		}
	}

	sl.threshold.OnCommand = func(v float32) {
		sl.threshold.Set(v)
		if err := write().Threshold(float64(v)); err != nil {
			slog.Error("saving the wake threshold failed", "slot", n+1, "err", err)
		}
		slog.Info("wake sensitivity", "slot", n+1, "cutoff", v)
	}
	sl.tone.OnCommand = func(label string) {
		tone, ok := s.toneOf(label)
		if !ok {
			slog.Warn("unknown wake tone", "slot", n+1, "value", label)
			return
		}
		sl.tone.Set(label)
		if err := write().Tone(tone); err != nil {
			slog.Error("saving the wake tone failed", "slot", n+1, "err", err)
		}
	}
	sl.delivery.OnCommand = func(label string) {
		how, ok := byLabel(schema.Deliveries(), label)
		if !ok {
			slog.Warn("unknown reply delivery", "slot", n+1, "value", label)
			return
		}
		sl.delivery.Set(how.Label())
		if err := write().Delivery(how); err != nil {
			slog.Error("saving the reply delivery failed", "slot", n+1, "err", err)
		}
		slog.Info("reply delivery", "slot", n+1, "using", how)
	}
	sl.buffer.OnCommand = func(v float32) {
		sl.buffer.Set(v)
		if err := write().Buffer(int(v)); err != nil {
			slog.Error("saving the reply buffer failed", "slot", n+1, "err", err)
		}
	}
	sl.followUp.OnCommand = func(v float32) {
		sl.followUp.Set(v)
		if err := write().FollowUp(int(v)); err != nil {
			slog.Error("saving the follow-up time failed", "slot", n+1, "err", err)
		}
	}
	sl.maxListen.OnCommand = func(v float32) {
		sl.maxListen.Set(v)
		if err := write().MaxListen(int(v)); err != nil {
			slog.Error("saving the listening limit failed", "slot", n+1, "err", err)
		}
	}
	sl.maxThink.OnCommand = func(v float32) {
		sl.maxThink.Set(v)
		if err := write().MaxThink(int(v)); err != nil {
			slog.Error("saving the thinking limit failed", "slot", n+1, "err", err)
		}
	}
	sl.wakeScreen.OnCommand = func(on bool) {
		sl.wakeScreen.Set(on)
		if err := write().WakeScreen(on); err != nil {
			slog.Error("saving wake the screen failed", "slot", n+1, "err", err)
		}
	}
	return sl
}

func (s *Settings) Name() string { return "wake word settings" }

func (s *Settings) Entities() []esphome.Entity {
	var ents []esphome.Entity
	for n, sl := range s.slots {
		ents = append(ents, sl.wake, sl.threshold, sl.tone)
		if s.options.Extras != nil {
			ents = append(ents, s.options.Extras(n)...)
		}
		ents = append(ents, sl.delivery, sl.buffer, sl.followUp, sl.maxListen, sl.maxThink)
		if s.options.Screen {
			ents = append(ents, sl.wakeScreen)
		}
	}
	return ents
}

func (s *Settings) Restore(words []schema.WakeWord) {
	for i, sl := range s.slots {
		saved := schema.DefaultWakeWord()
		if i < len(words) {
			saved = words[i]
		}

		sl.threshold.Set(float32(saved.Threshold))
		sl.tone.Set(s.toneLabel(saved.Tone))
		sl.delivery.Set(saved.Delivery.Label())
		sl.buffer.Set(float32(saved.Buffer))
		sl.followUp.Set(float32(saved.FollowUp))
		sl.maxListen.Set(float32(saved.MaxListen))
		sl.maxThink.Set(float32(saved.MaxThink))
		sl.wakeScreen.Set(saved.WakeScreen)
	}
	slog.Info("restored", "what", "wake word settings", "slots", len(s.slots))
}

func (s *Settings) Refresh(words []schema.WakeWord) {
	for _, sl := range s.slots {
		sl.tone.SetOptions(s.toneLabels())
	}
	s.Restore(words)
}

func (s *Settings) tones() []setting.Option {
	if s.options.Tones == nil {
		return nil
	}
	return s.options.Tones()
}

func (s *Settings) toneLabels() []string {
	out := labels(schema.Chimes())
	for _, o := range s.tones() {
		out = append(out, o.Label)
	}
	return out
}

func (s *Settings) toneLabel(c schema.Chime) string {
	for _, o := range s.tones() {
		if o.Value == string(c) {
			return o.Label
		}
	}
	return c.Label()
}

func (s *Settings) toneOf(label string) (schema.Chime, bool) {
	if c, ok := byLabel(schema.Chimes(), label); ok {
		return c, true
	}
	for _, o := range s.tones() {
		if o.Label == label {
			return schema.Chime(o.Value), true
		}
	}
	return "", false
}

type labelled interface{ Label() string }

func labels[T labelled](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v.Label()
	}
	return out
}

func byLabel[T labelled](values []T, label string) (T, bool) {
	for _, v := range values {
		if v.Label() == label {
			return v, true
		}
	}
	var zero T
	return zero, false
}
