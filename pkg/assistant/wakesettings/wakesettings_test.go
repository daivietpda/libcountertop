package wakesettings

import (
	"slices"
	"testing"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type memory struct{ wake schema.Wake }

func (m *memory) Read() schema.Wake { return m.wake }
func (m *memory) Update(change func(*schema.Wake)) error {
	change(&m.wake)
	return nil
}

func TestACommandIsSavedToItsSlot(t *testing.T) {
	m := &memory{}
	s := New(Options{
		Slots:    2,
		DeviceID: func(slot int) uint32 { return uint32(10 + slot) },
		Write:    func(slot int) schema.WakeWriter { return schema.NewWakeWriter(m, slot) },
	})

	s.slots[1].threshold.OnCommand(0.6)
	s.slots[1].delivery.OnCommand(schema.DeliveryStream.Label())

	got := m.wake.Slot(1)
	if got.Threshold != float64(float32(0.6)) {
		t.Errorf("threshold %v, want 0.6", got.Threshold)
	}
	if got.Delivery != schema.DeliveryStream {
		t.Errorf("delivery %v, want stream", got.Delivery)
	}
	if m.wake.Slot(0).Threshold != schema.DefaultThreshold {
		t.Errorf("the first slot changed: %+v", m.wake.Slot(0))
	}
	if id := s.slots[1].tone.DeviceID; id != 11 {
		t.Errorf("device %d, want 11", id)
	}
}

func TestExtrasFollowTheToneOfTheirSlot(t *testing.T) {
	extra := []*esphome.Select{
		{Base: esphome.Base{ObjectID: "extra_1"}},
		{Base: esphome.Base{ObjectID: "extra_2"}},
	}
	s := New(Options{
		Slots:  2,
		Extras: func(slot int) []esphome.Entity { return []esphome.Entity{extra[slot]} },
	})

	ents := s.Entities()
	if len(ents) != 18 {
		t.Fatalf("%d entities, want 18", len(ents))
	}
	if ents[3] != extra[0] || ents[12] != extra[1] {
		t.Errorf("extras at the wrong place: %v %v", ents[3], ents[12])
	}
}

func TestWakeTheScreenIsOfferedOnlyWithAScreen(t *testing.T) {
	m := &memory{}
	write := func(slot int) schema.WakeWriter { return schema.NewWakeWriter(m, slot) }
	blind := New(Options{Slots: 2, Read: m.Read, Write: write})
	seeing := New(Options{Slots: 2, Screen: true, Read: m.Read, Write: write})

	if got, want := len(seeing.Entities()), len(blind.Entities())+2; got != want {
		t.Fatalf("%d entities with a screen, want %d", got, want)
	}
	for _, e := range blind.Entities() {
		if _, ok := e.(*esphome.Switch); ok {
			t.Errorf("a device without a screen offers %T", e)
		}
	}
	if slices.Contains(blind.Controls().Names(), "2.wake_screen") {
		t.Error("a device without a screen has a wake_screen row")
	}

	seeing.slots[1].wakeScreen.OnCommand(true)
	if !m.wake.Slot(1).WakeScreen || m.wake.Slot(0).WakeScreen {
		t.Errorf("saved %+v", m.wake.Words)
	}
	if err := seeing.Controls().Change("1.wake_screen", "on"); err != nil {
		t.Fatal(err)
	}
	if !m.wake.Slot(0).WakeScreen {
		t.Error("the row did not turn assistant 1 on")
	}
}
