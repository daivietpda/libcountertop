package widgets

import "testing"

func TestShrinkKeepsWhatFits(t *testing.T) {
	if got := Shrink(40, 1000, func(size float32) float32 { return size * 10 }); got != 40 {
		t.Errorf("text with room to spare shrank to %v", got)
	}
}

func TestShrinkFitsTheRoom(t *testing.T) {
	wide := func(size float32) float32 { return size*10 + 7 }
	got := Shrink(40, 250, wide)
	if wide(got) > 250 {
		t.Errorf("shrank to %v, which is %v wide in a room of 250", got, wide(got))
	}
	if got < 23 {
		t.Errorf("shrank to %v, further than the room needs", got)
	}
}

func TestShrinkStopsAtOne(t *testing.T) {
	if got := Shrink(40, 1, func(size float32) float32 { return size * 10 }); got != 1 {
		t.Errorf("text that can never fit shrank to %v, want 1", got)
	}
}
